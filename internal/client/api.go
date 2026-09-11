package client

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/model"
)

func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

type APIError struct {
	Status  int
	Code    string `json:"code"`
	Message string `json:"message"`
	Result  model.Result
}

func (e *APIError) Error() string {
	return fmt.Sprintf("control API %d (%s)", e.Status, e.Code)
}
func (e *APIError) Unwrap() error {
	return &model.BusinessError{Code: e.Code, Details: e.Result.Details}
}

func IdentityEnded(err error) bool   { return model.EndsIdentity(model.Code(err)) }
func MembershipEnded(err error) bool { return model.EndsMembership(model.Code(err)) }
func NodeAuthorizationEnded(err error) bool {
	return model.IsCode(err, "node_revoked", "node_generation_stale", "node_not_authorized", "node_disabled")
}
func endsMachineIdentity(err error) bool {
	return IdentityEnded(err) || model.IsCode(err, "node_revoked", "node_generation_stale", "node_not_authorized")
}

type intent struct {
	Key      string
	Deadline time.Time
}
type intentKey struct{}

func WithOperation(ctx context.Context, key string, deadline time.Time) context.Context {
	return context.WithValue(ctx, intentKey{}, intent{key, deadline})
}

type API struct {
	Identity device.Identity
	HTTP     *http.Client
	mu       sync.Mutex
	session  model.Session
	terminal error
}

func NewAPI(i device.Identity) *API {
	return &API{Identity: i, HTTP: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (a *API) RegisterGuest(ctx context.Context) error {
	a.mu.Lock()
	a.Identity.PendingGuest = true
	a.mu.Unlock()
	return a.Authenticate(ctx)
}
func (a *API) Account() *model.User {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session.User == nil {
		return nil
	}
	u := *a.session.User
	return &u
}
func (a *API) Authenticate(ctx context.Context) error { _, err := a.login(ctx); return err }
func (a *API) Reauthenticate(ctx context.Context) error {
	a.mu.Lock()
	a.session = model.Session{}
	a.terminal = nil
	a.mu.Unlock()
	return a.Authenticate(ctx)
}
func (a *API) login(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.terminal != nil {
		return "", a.terminal
	}
	if a.session.Token != "" && time.Until(a.session.ExpiresAt) > time.Minute {
		return a.session.Token, nil
	}
	if len(a.Identity.PrivateKey) != ed25519.PrivateKeySize {
		return "", errors.New("invalid device identity")
	}
	pub := ed25519.PrivateKey(a.Identity.PrivateKey).Public().(ed25519.PublicKey)
	scope, prefix := "player", "/v2/auth"
	if a.Identity.PendingGuest {
		scope, prefix = "guest", "/v2/auth/guest"
	}
	if a.Identity.Node {
		scope, prefix = "node", "/v2/node/auth"
	}
	var challenge model.Challenge
	if err := a.request(ctx, "POST", prefix+"/challenge", model.ChallengeRequest{DeviceID: a.Identity.ID(), Name: a.Identity.Name, PublicKey: pub}, &challenge, "", ID()); err != nil {
		if endsMachineIdentity(err) {
			a.terminal = err
		}
		return "", err
	}
	message := append([]byte("nodelane-auth-v2:"+scope+":"+challenge.ID+":"), challenge.Nonce...)
	sig := ed25519.Sign(a.Identity.PrivateKey, message)
	if err := a.request(ctx, "POST", prefix+"/verify", model.VerifyRequest{ID: challenge.ID, Signature: sig}, &a.session, "", ID()); err != nil {
		if endsMachineIdentity(err) {
			a.terminal = err
		}
		return "", err
	}
	if a.session.DeviceID != a.Identity.ID() {
		a.session = model.Session{}
		return "", model.Failure("local_control_response_invalid")
	}
	a.Identity.PendingGuest = false
	return a.session.Token, nil
}
func (a *API) Call(ctx context.Context, method, path string, in, out any) error {
	result, err := a.CallResult(ctx, method, path, in)
	if err != nil {
		return err
	}
	if out != nil && len(result.Data) > 0 {
		if err = decodeResponse(result.Data, out); err != nil {
			return model.Failure("local_control_response_invalid")
		}
	}
	return nil
}

func (a *API) CallResult(ctx context.Context, method, path string, in any) (model.Result, error) {
	op, ok := ctx.Value(intentKey{}).(intent)
	if !ok {
		op = intent{ID(), time.Now().UTC().Add(time.Hour)}
	}
	ctx = WithOperation(ctx, op.Key, op.Deadline)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var body []byte
	var marshalErr error
	if raw, ok := in.(json.RawMessage); ok {
		body = append([]byte(nil), raw...)
	} else if in != nil {
		body, marshalErr = json.Marshal(in)
	}
	if marshalErr != nil {
		return model.Result{}, model.Failure("local_internal_error")
	}
	var err error
	var result model.Result
	reauthenticated := false
	for attempt := 0; attempt < 3; attempt++ {
		token, e := a.login(ctx)
		if e != nil {
			return result, e
		}
		result, err = a.requestResult(ctx, method, path, body, token, op.Key)
		if err == nil {
			return result, nil
		}
		if endsMachineIdentity(err) && !model.IsCode(err, "node_generation_stale") {
			a.mu.Lock()
			a.terminal = err
			a.session = model.Session{}
			a.mu.Unlock()
			return result, err
		}
		var apierr *APIError
		if errors.As(err, &apierr) {
			if !reauthenticated && (apierr.Code == "auth_session_required" || apierr.Code == "node_session_required") {
				reauthenticated = true
				a.mu.Lock()
				if a.session.Token == token {
					a.session = model.Session{}
				}
				a.mu.Unlock()
				continue
			}
			if apierr.Status < 500 && apierr.Code != "request_rate_limited" {
				return result, err
			}
		}
		if model.IsCode(err, "local_tls_failed", "local_request_cancelled") {
			return result, err
		}
		if attempt == 2 {
			break
		}
		delay := time.Duration(attempt+1) * 250 * time.Millisecond
		if apierr != nil && apierr.Code == "request_rate_limited" {
			delay = time.Duration(apierr.Result.Retry.AfterMS) * time.Millisecond
			if delay <= 0 {
				delay = time.Minute
			}
		}
		if until, ok := ctx.Deadline(); ok && time.Until(until) <= delay {
			return result, err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return result, transportFailure(ctx.Err())
		case <-timer.C:
		}
	}
	return result, err
}
func (a *API) request(ctx context.Context, method, path string, in, out any, token, key string) error {
	var body []byte
	var err error
	if in != nil {
		body, err = json.Marshal(in)
		if err != nil {
			return err
		}
	}
	result, err := a.requestResult(ctx, method, path, body, token, key)
	if err != nil {
		return err
	}
	if out != nil {
		return decodeResponse(result.Data, out)
	}
	return nil
}

func (a *API) requestResult(ctx context.Context, method, path string, body []byte, token, key string) (model.Result, error) {
	var result model.Result
	r, err := http.NewRequestWithContext(ctx, method, a.Identity.Server+path, bytes.NewReader(body))
	if err != nil {
		return result, transportFailure(err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(model.ContractHeader, model.Contract)
	r.Header.Set("Idempotency-Key", key)
	op, ok := ctx.Value(intentKey{}).(intent)
	if !ok {
		op.Deadline = time.Now().UTC().Add(time.Hour)
	}
	r.Header.Set(model.DeadlineHeader, op.Deadline.Format(time.RFC3339Nano))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := a.HTTP.Do(r)
	if err != nil {
		return result, transportFailure(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil {
		return result, model.Failure("local_control_connection_lost")
	}
	if len(b) > 4<<20 || json.Unmarshal(b, &result) != nil || result.Contract != model.Contract || result.Code == "" || result.RequestID == "" {
		return result, &APIError{Status: resp.StatusCode, Code: "local_control_response_invalid"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, &APIError{Status: resp.StatusCode, Code: result.Code, Message: result.Message, Result: result}
	}
	if _, ok := model.BusinessCodes[result.Code]; !ok || model.HTTPStatus(result.Code) >= 400 {
		return result, &APIError{Status: resp.StatusCode, Code: "local_control_response_invalid"}
	}
	result.ControlHTTPStatus = &resp.StatusCode
	return result, nil
}

func transportFailure(err error) error {
	var dns *net.DNSError
	var verify *tls.CertificateVerificationError
	var authority x509.UnknownAuthorityError
	var n net.Error
	code := "local_control_unreachable"
	switch {
	case errors.Is(err, context.Canceled):
		code = "local_request_cancelled"
	case errors.As(err, &verify), errors.As(err, &authority):
		code = "local_tls_failed"
	case errors.As(err, &dns):
		code = "local_dns_failed"
	case errors.Is(err, context.DeadlineExceeded):
		code = "local_control_timeout"
	case errors.As(err, &n):
		if n.Timeout() {
			code = "local_control_timeout"
		}
	}
	return model.Failure(code)
}
func (a *API) Watch(ctx context.Context, room string, after int64, onSnapshot func(model.Snapshot)) error {
	token, err := a.login(ctx)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, "GET", a.Identity.Server+"/v2/rooms/"+room+"/events", nil)
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set(model.ContractHeader, model.Contract)
	r.Header.Set("Last-Event-ID", fmt.Sprint(after))
	h := *a.HTTP
	h.Timeout = 0
	resp, err := h.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var result model.Result
		if json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&result) != nil || result.Contract != model.Contract {
			return model.Failure("local_control_response_invalid")
		}
		return &APIError{Status: resp.StatusCode, Code: result.Code, Result: result}
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	kind := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			kind = strings.TrimPrefix(line, "event: ")
		}
		if kind == "snapshot" && strings.HasPrefix(line, "data: ") {
			var s model.Snapshot
			if err = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &s); err != nil {
				return err
			}
			onSnapshot(s)
		}
		if kind == "terminal" && strings.HasPrefix(line, "data: ") {
			var result model.Result
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &result) != nil {
				return model.Failure("local_control_response_invalid")
			}
			return &APIError{Code: result.Code, Result: result}
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

func decodeResponse(data []byte, out any) error {
	target := reflect.ValueOf(out)
	if target.Kind() != reflect.Pointer || target.IsNil() {
		return model.Failure("local_internal_error")
	}
	next := reflect.New(target.Elem().Type())
	if err := json.Unmarshal(data, next.Interface()); err != nil {
		return model.Failure("local_control_response_invalid")
	}
	valid := true
	switch value := next.Interface().(type) {
	case *model.Challenge:
		valid = len(value.ID) == 32 && len(value.Nonce) == 32
	case *model.Session:
		valid = len(value.Token) == 64 && value.DeviceID != "" && !value.ExpiresAt.IsZero() && (value.NodeID == "" || value.Generation > 0)
	case *model.AccountStatus:
		valid = value.User.ID != "" && value.Device.DeviceID != "" && (value.Membership.State == "none" || ((value.Membership.State == "active" || value.Membership.State == "ended") && value.Membership.RoomID != ""))
	case *model.Snapshot:
		valid = !value.ServerTime.IsZero()
	case *model.RoomPage:
		valid = value.Rooms != nil
	}
	if !valid {
		return model.Failure("local_control_response_invalid")
	}
	target.Elem().Set(next.Elem())
	return nil
}
