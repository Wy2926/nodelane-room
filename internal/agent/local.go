package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method != "POST" || req.URL.Path != "/rpc" {
			http.NotFound(w, req)
			return
		}
		var in localapi.Request
		d := json.NewDecoder(http.MaxBytesReader(w, req.Body, 65536))
		d.DisallowUnknownFields()
		if err := d.Decode(&in); err != nil {
			http.Error(w, "invalid RPC request", 400)
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
			err = errors.New("unknown action")
		}
		if err != nil {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
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
