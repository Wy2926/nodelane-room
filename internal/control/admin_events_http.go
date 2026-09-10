package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request, _ string) {
	out, err := s.adminSnapshot(r.Context())
	s.result(w, out, err)
}

func (s *Server) adminEvents(w http.ResponseWriter, r *http.Request, _ string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	if r.Method == http.MethodHead {
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		return
	}
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	// Each snapshot carries a durable cursor and recent audit events. A stale or
	// future Last-Event-ID always recovers the complete current state.
	for {
		c, err := r.Cookie(adminCookie)
		if err != nil {
			return
		}
		var valid bool
		if err = s.Store.Pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM admin_sessions WHERE token_hash=$1 AND expires_at>now() AND last_seen>now()-interval '30 minutes')", hash(c.Value)).Scan(&valid); err != nil || !valid {
			return
		}
		out, err := s.adminSnapshot(r.Context())
		if err != nil {
			return
		}
		b, err := json.Marshal(out)
		if err != nil {
			return
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err = fmt.Fprintf(w, "id: %d\nevent: snapshot\ndata: %s\n\n", out.Revision, b); err != nil {
			return
		}
		f.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			return
		case <-timer.C:
		}
	}
}
