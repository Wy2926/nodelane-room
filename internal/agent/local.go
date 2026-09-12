package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/localapi"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
)

func (r *Runtime) Serve(ctx context.Context) error {
	l, err := platform.ListenLocal(r.dir)
	if err != nil {
		return err
	}
	defer l.Close()
	srv := &http.Server{Handler: r.localHandler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 30 * time.Second}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Run(runCtx) }()
	go func() { <-runCtx.Done(); _ = srv.Close() }()
	err = srv.Serve(l)
	cancel()
	<-done
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (r *Runtime) localHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", client.ID())
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !r.nodeMode && req.Method == "GET" && strings.HasPrefix(req.URL.Path, "/game-images/") {
			parts := strings.Split(strings.TrimPrefix(req.URL.Path, "/game-images/"), "/")
			if len(parts) != 2 {
				localError(w, localapi.Failure("request_validation_failed", "invalid game image"))
				return
			}
			b, mime, err := r.gameImage(req.Context(), parts[0], parts[1])
			if err != nil {
				localError(w, err)
				return
			}
			w.Header().Set("Content-Type", mime)
			_, _ = w.Write(b)
			return
		}
		if req.Method != "POST" || req.URL.Path != "/rpc" {
			http.NotFound(w, req)
			return
		}
		var in localapi.Request
		d := json.NewDecoder(http.MaxBytesReader(w, req.Body, 65536))
		d.DisallowUnknownFields()
		if err := d.Decode(&in); err != nil {
			localError(w, model.Failure("request_malformed"))
			return
		}
		if d.Decode(&struct{}{}) != io.EOF || !validLocalID(in.Room, true) {
			localError(w, model.Failure("request_malformed"))
			return
		}
		if in.Contract != model.Contract {
			localError(w, model.Failure("local_protocol_incompatible"))
			return
		}
		w.Header().Set("X-Operation-ID", in.CommandID)
		if len(in.Body) == 0 {
			in.Body = json.RawMessage("{}")
		}
		if r.nodeMode {
			out, err := r.nodeLocal(req.Context(), in)
			if err != nil {
				localError(w, err)
				return
			}
			localResult(w, in.CommandID, out)
			return
		}

		var out any
		var err error
		if commandWrites(in.Action) {
			out, err = r.runCommand(req.Context(), in)
		} else {
			out, err = r.dispatch(req.Context(), in)
		}
		if err != nil {
			localError(w, err)
			return
		}
		localResult(w, in.CommandID, out)
	})
}

func (r *Runtime) dispatch(ctx context.Context, in localapi.Request) (any, error) {
	var out any
	var err error
	switch in.Action {
	case "get-operation":
		return r.getOperation(ctx, in.Target)
	case "network-stop", "network-retry":
		return r.networkAction(ctx, in.Action)
	case "capabilities", "invite-info", "account-devices", "account-status":
		return r.readInteraction(ctx, in)

	case "update-check", "update-status", "update-install", "update-download", "update-cancel", "update-cancel-install":
		out, err = r.updateAction(ctx, in)
	case "account-login", "account-link", "account-poll", "account-cancel", "account-logout", "account-takeover", "revoke-device":
		out, err = r.accountAction(ctx, in)
	case "init":
		out, err = r.Init(ctx, in.Server, in.Name)
	case "status":
		var report struct {
			GUIVersion string `json:"gui_version"`
		}
		if json.Unmarshal(in.Body, &report) == nil && model.ValidVersion(report.GUIVersion) {
			r.updateMu.Lock()
			r.guiVersion = report.GUIVersion
			r.updateMu.Unlock()
		}
		out = r.Status()
	case "doctor":
		out = r.Doctor()
	case "members":
		out, err = r.Members(ctx, in.Room)
	case "rooms":
		out, err = r.OwnedRooms(ctx)
	case "manage":
		out, err = r.ManageRoom(ctx, in.Room)
	case "games":
		out, err = r.Games(ctx)
	case "ping":
		out, err = r.Ping(ctx, in.Target)

	case "create", "join", "owner-join", "invite", "invite-revoke", "kick", "transfer", "leave", "close":
		out, err = r.Action(ctx, in.Action, in.Room, in.Body)
	default:
		err = localapi.Failure("request_method_unsupported", "unknown player action")
	}
	return out, err
}

func serviceFailure(err error, operation string) model.Result {
	code := model.Code(err)
	if code == "system_internal_error" {
		code = "local_internal_error"
	}
	out := model.NewResult(code, "service", client.ID(), nil)
	var remote *client.APIError
	var business *model.BusinessError
	var network net.Error
	switch {
	case errors.As(err, &remote):
		if remote.Result.Contract == model.Contract {
			out = remote.Result
		}
		out.Code = remote.Code
		out.Origin = "control"
		if strings.HasPrefix(remote.Code, "local_") {
			out.Origin = "service"
		}
		out.CauseRequestID = remote.Result.RequestID
		if remote.Status > 0 {
			out.ControlHTTPStatus = &remote.Status
		}
		out.RequestID = client.ID()
	case errors.As(err, &business):
		out.Details = business.Details
	case errors.Is(err, context.DeadlineExceeded):
		out = model.NewResult("local_control_timeout", "service", client.ID(), nil)
	case errors.As(err, &network):
		out = model.NewResult("local_control_unreachable", "service", client.ID(), nil)
	}
	out.OperationID = operation
	if spec, ok := model.BusinessCodes[out.Code]; ok {
		out.Message = spec.Message
	} else {
		out.Message = "服务返回了暂不支持的结果，请凭请求编号查询"
	}
	out.Data = nil
	out.Details = model.SafeDetails(out.Code, out.Details)
	now := time.Now().UTC()
	out.ObservedAt = &now
	return out
}

func localError(w http.ResponseWriter, err error) {
	out := serviceFailure(err, w.Header().Get("X-Operation-ID"))
	if id := w.Header().Get("X-Request-ID"); id != "" {
		out.RequestID = id
	}
	w.WriteHeader(model.HTTPStatus(out.Code))
	_ = json.NewEncoder(w).Encode(out)
}

func localResult(w http.ResponseWriter, operation string, value any) {
	out := model.NewResult("ok", "service", w.Header().Get("X-Request-ID"), value)
	if result, ok := value.(model.Result); ok {
		out = result
		if out.Origin == "control" {
			out.CauseRequestID = out.RequestID
		}
	}
	out.RequestID = w.Header().Get("X-Request-ID")
	out.OperationID = operation
	if spec, ok := model.BusinessCodes[out.Code]; ok {
		out.Message = spec.Message
	} else {
		out.Message = "服务返回了暂不支持的结果，请凭请求编号查询"
	}
	out.Details = model.SafeDetails(out.Code, out.Details)
	now := time.Now().UTC()
	out.ObservedAt = &now
	w.WriteHeader(model.HTTPStatus(out.Code))
	_ = json.NewEncoder(w).Encode(out)
}

func validLocalID(id string, empty bool) bool {
	if id == "" {
		return empty
	}
	if len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
