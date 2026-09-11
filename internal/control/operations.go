package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

type operationContext struct {
	ID, Method, Path, RequestID string
	Deadline                    time.Time
}
type operationContextKey struct{}

func operationID(ctx context.Context) string {
	op, _ := ctx.Value(operationContextKey{}).(operationContext)
	return op.ID
}

func requestOperation(r *http.Request, requestID string) *http.Request {
	deadline, _ := time.Parse(time.RFC3339Nano, r.Header.Get(model.DeadlineHeader))
	op := operationContext{ID: r.Header.Get("Idempotency-Key"), Method: r.Method, Path: r.URL.Path, RequestID: requestID, Deadline: deadline}
	return r.WithContext(context.WithValue(r.Context(), operationContextKey{}, op))
}

func (s *Store) mutateChecked(ctx context.Context, device, key, requestHash string, check func(pgx.Tx) error, fn func(pgx.Tx) (any, error)) ([]byte, error) {
	if len(key) < 16 || len(key) > 128 {
		return nil, model.Failure("request_idempotency_required")
	}
	op, wire := ctx.Value(operationContextKey{}).(operationContext)
	if wire {
		requestHash = hash(requestHash + ":" + op.Deadline.UTC().Format(time.RFC3339Nano))
	}
	var receipt model.Receipt
	err := s.Write(ctx, func(tx pgx.Tx) error {
		if check != nil {
			if err := check(tx); err != nil {
				return err
			}
		}
		var now time.Time
		if err := tx.QueryRow(ctx, "SELECT now()").Scan(&now); err != nil {
			return err
		}
		if !wire {
			op.Deadline = now.Add(time.Hour)
		}
		if op.Deadline.IsZero() || op.Deadline.After(now.Add(time.Hour)) {
			return model.Validation("operation_deadline", "out_of_range")
		}
		if !op.Deadline.After(now) {
			return model.Failure("operation_expired")
		}
		var previous string
		var raw []byte
		err := tx.QueryRow(ctx, "SELECT request_hash,response FROM idempotency WHERE device_id=$1 AND key=$2 AND expires_at>now()", device, key).Scan(&previous, &raw)
		if err == nil {
			if previous != requestHash {
				return model.Failure("request_idempotency_conflict")
			}
			if err = json.Unmarshal(raw, &receipt); err != nil {
				return err
			}
			return visibleReceipt(ctx, tx, device, &receipt)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		business, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		value, err := fn(business)
		result := model.NewResult("ok", "control", op.RequestID, value)
		if v, ok := value.(model.Result); ok {
			result = v
		}
		status := model.HTTPStatus(result.Code)
		if op.Method == "POST" && (op.Path == "/v2/rooms" || op.Path == "/v2/admin/nodes") {
			status = 201
		}
		if err != nil {
			if rollbackErr := business.Rollback(ctx); rollbackErr != nil {
				return rollbackErr
			}
			var failure *model.BusinessError
			if !errors.As(err, &failure) {
				return err
			}
			if model.HTTPStatus(failure.Code) >= 500 || model.HTTPStatus(failure.Code) == 401 || model.HTTPStatus(failure.Code) == 429 {
				return err
			}
			result = model.NewResult(failure.Code, "control", op.RequestID, nil)
			result.Details = model.SafeDetails(failure.Code, failure.Details)
			status = model.HTTPStatus(failure.Code)
		} else if err = business.Commit(ctx); err != nil {
			return err
		}
		result.OperationID = key
		result.ServerTime = &now
		receipt = model.Receipt{Method: op.Method, Path: op.Path, Deadline: op.Deadline, Status: status, Result: result}
		raw, err = json.Marshal(receipt)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO idempotency(device_id,key,request_hash,response,expires_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(device_id,key) DO UPDATE SET request_hash=EXCLUDED.request_hash,response=EXCLUDED.response,expires_at=EXCLUDED.expires_at`, device, key, requestHash, raw, op.Deadline)
		return err
	})
	if err != nil {
		return nil, err
	}
	if !wire {
		if receipt.Status >= 400 {
			return nil, &model.BusinessError{Code: receipt.Result.Code, Details: receipt.Result.Details}
		}
		return receipt.Result.Data, nil
	}
	return json.Marshal(receipt)
}

func visibleReceipt(ctx context.Context, tx pgx.Tx, device string, receipt *model.Receipt) error {
	if receipt.Status >= 400 || strings.HasPrefix(device, "admin:") {
		return nil
	}
	var value struct {
		Room        *model.Room       `json:"room"`
		Invitation  *model.Invitation `json:"invitation"`
		Code        string            `json:"code"`
		Fingerprint string            `json:"fingerprint"`
		ID          string            `json:"id"`
	}
	if err := json.Unmarshal(receipt.Result.Data, &value); err != nil {
		return nil
	}
	if value.Fingerprint != "" {
		var valid bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM certificates WHERE fingerprint=$1 AND device_id=$2 AND NOT revoked AND expires_at>now())", value.Fingerprint, device).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return model.Failure("lease_replay_obsolete")
		}
	}
	room := ""
	if value.Room != nil {
		room = value.Room.ID
	}
	parts := strings.Split(receipt.Path, "/")
	if len(parts) >= 4 && parts[2] == "rooms" && parts[3] != "join" {
		room = parts[3]
	}
	if room == "" {
		return nil
	}
	var owner, member bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rooms WHERE id=$1 AND owner_user_id=(SELECT user_id FROM user_devices WHERE device_id=$2) AND NOT closed AND expires_at>now()),EXISTS(SELECT 1 FROM members m JOIN rooms r ON r.id=m.room_id WHERE room_id=$1 AND device_id=$2 AND active AND NOT r.closed AND r.expires_at>now())`, room, device).Scan(&owner, &member); err != nil {
		return err
	}
	code := value.Code
	if value.Invitation != nil {
		code = value.Invitation.Code
	}
	valid := owner || member
	if code != "" {
		valid = false
		if owner {
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM invitations WHERE room_id=$1 AND code_hash=$2 AND expires_at>now())", room, hash(code)).Scan(&valid); err != nil {
				return err
			}
		}
	}
	if !valid {
		old := receipt.Result
		receipt.Result = model.NewResult("operation_result_redacted", "control", old.RequestID, nil)
		receipt.Result.OperationID = old.OperationID
		receipt.Result.ServerTime = old.ServerTime
		receipt.Status = 200
	}
	return nil
}

func (s *Server) operationReceipt(w http.ResponseWriter, r *http.Request, subject string) {
	tx, err := s.Store.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if strings.HasPrefix(subject, "admin:") {
		cookie, _ := r.Cookie(adminCookie)
		if cookie == nil {
			s.fail(w, model.Failure("admin_session_required"))
			return
		}
		err = validAdminSession(r.Context(), tx, hash(cookie.Value))
	} else {
		err = validPlayerSession(r.Context(), tx, subject, hash(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")))
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	var raw []byte
	var expired bool
	err = tx.QueryRow(r.Context(), "SELECT response,expires_at<=now() FROM idempotency WHERE device_id=$1 AND key=$2", subject, r.PathValue("operation")).Scan(&raw, &expired)
	if errors.Is(err, pgx.ErrNoRows) {
		s.fail(w, model.Failure("operation_not_found"))
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if expired {
		s.fail(w, model.Failure("operation_expired"))
		return
	}
	var receipt model.Receipt
	if err = json.Unmarshal(raw, &receipt); err == nil {
		err = visibleReceipt(r.Context(), tx, subject, &receipt)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	state := "succeeded"
	if receipt.Status >= 400 {
		state = "rejected"
	}
	s.result(w, model.Operation{ID: r.PathValue("operation"), State: state, Deadline: receipt.Deadline, KnownCommit: receipt.Status < 400, Result: &receipt.Result}, nil)
}
