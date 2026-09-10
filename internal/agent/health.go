package agent

import (
	"fmt"
	"net/http"
)

func (r *Runtime) HealthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		s := r.Status()
		if s.Engine != "running" {
			w.WriteHeader(503)
			fmt.Fprintln(w, "not connected")
			return
		}
		fmt.Fprintln(w, "ready")
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		s := r.Status()
		running, control := 0, 0
		if s.Engine == "running" {
			running = 1
		}
		if s.Control == "connected" {
			control = 1
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "nodelane_engine_running %d\nnodelane_control_connected %d\nnodelane_lease_expiry_timestamp_seconds %d\n", running, control, max(0, s.LeaseExpiresAt.Unix()))
	})
	return mux
}
