package agent

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/nodelane/nodelane-room/internal/client"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

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
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !r.nodeMode && req.Method == "GET" && strings.HasPrefix(req.URL.Path, "/game-images/") {
			parts := strings.Split(strings.TrimPrefix(req.URL.Path, "/game-images/"), "/")
			if len(parts) != 2 {
				localError(w, localapi.Failure("invalid_request", "invalid game image"))
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
			localError(w, localapi.Failure("invalid_request", "invalid RPC request"))
			return
		}
		if d.Decode(&struct{}{}) != io.EOF || !validLocalID(in.Room, true) {
			localError(w, localapi.Failure("invalid_request", "invalid RPC request"))
			return
		}
		if len(in.Body) == 0 {
			in.Body = json.RawMessage("{}")
		}
		if r.nodeMode {
			out, err := r.nodeLocal(req.Context(), in)
			if err != nil {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(out)
			return
		}
		var out any
		var err error
		switch in.Action {
		case "init":
			out, err = r.Init(req.Context(), in.Server, in.Name)
		case "status":
			out = r.Status()
		case "doctor":
			out = r.Doctor()
		case "members":
			out, err = r.Members(req.Context(), in.Room)
		case "rooms":
			out, err = r.OwnedRooms(req.Context())
		case "manage":
			out, err = r.ManageRoom(req.Context(), in.Room)
		case "games":
			out, err = r.Games(req.Context())
		case "ping":
			out, err = r.Ping(req.Context(), in.Target)
		case "port", "remove-port":
			var p model.EndpointRequest
			err = json.Unmarshal(in.Body, &p)
			if err == nil {
				err = r.SetPort(req.Context(), p, in.Action == "remove-port")
			}
			out = map[string]bool{"ok": err == nil}
		case "create", "join", "invite", "kick", "transfer", "leave", "close":
			out, err = r.Action(req.Context(), in.Action, in.Room, in.Body)
		default:
			err = localapi.Failure("unknown_action", "unknown player action")
		}
		if err != nil {
			localError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	})
}

func localError(w http.ResponseWriter, err error) {
	out := localapi.Error{Code: "operation_failed", Message: err.Error()}
	var local *localapi.Error
	var remote *client.APIError
	var network net.Error
	switch {
	case errors.As(err, &local):
		out = *local
	case errors.As(err, &remote):
		out.Code, out.Message = remote.Code, remote.Error()
	case errors.Is(err, context.DeadlineExceeded):
		out.Code = "timeout"
	case errors.As(err, &network):
		out.Code, out.Message = "control_unavailable", "无法连接控制服务，请检查网络与服务器地址"
	}
	w.WriteHeader(http.StatusBadRequest)
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
