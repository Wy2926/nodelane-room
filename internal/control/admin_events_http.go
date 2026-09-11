package control

import (
	"net/http"
	"strconv"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request, _ string) {
	out, err := s.adminSnapshot(r.Context())
	s.result(w, out, err)
}

func (s *Server) adminEvents(w http.ResponseWriter, r *http.Request, _ string) {
	initial, err := s.adminSnapshot(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	if r.Method == http.MethodHead {
		return
	}
	timer := time.NewTicker(5 * time.Second)
	defer timer.Stop()
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	terminal := func(code string) {
		out := model.NewResult(code, "control", randomID(), nil)
		out.ServerTime = &initial.ServerTime
		_ = writeSSE(w, "terminal", "", out)
	}
	// Each snapshot carries a durable cursor and recent audit events. A stale or
	// future Last-Event-ID always recovers the complete current state.
	for {
		c, err := r.Cookie(adminCookie)
		if err != nil {
			terminal("admin_session_required")
			return
		}
		var valid bool
		if err = s.Store.Pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM admin_sessions WHERE token_hash=$1 AND expires_at>now() AND last_seen>now()-interval '30 minutes')", hash(c.Value)).Scan(&valid); err != nil {
			terminal("system_unavailable")
			return
		}
		if !valid {
			terminal("admin_session_required")
			return
		}
		out, err := s.adminSnapshot(r.Context())
		if err != nil {
			terminal("system_unavailable")
			return
		}
		if err = writeSSE(w, "snapshot", strconv.FormatInt(out.Revision, 10), out); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			return
		case <-timer.C:
		}
	}
}
