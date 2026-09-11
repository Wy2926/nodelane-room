package control

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Server) playerEvents(w http.ResponseWriter, r *http.Request, id string) {
	room := r.PathValue("room")
	snapshot, err := s.Store.Snapshot(r.Context(), room, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	s.streams.Add(1)
	defer s.streams.Add(-1)
	send := func(kind string, rev int64, payload any) bool {
		return writeSSE(w, kind, strconv.FormatInt(rev, 10), payload) == nil
	}
	terminal := func(code string) {
		if code == "" {
			code = "member_revoked"
		}
		out := model.NewResult(code, "control", randomID(), snapshot.Self)
		out.Details.RoomID = room
		send("terminal", snapshot.Self.Revision, out)
	}
	if snapshot.Self.State != "active" {
		terminal(snapshot.Self.Reason)
		return
	}
	rev := snapshot.Self.Revision
	cursor := r.Header.Get("Last-Event-ID")
	last, parseErr := strconv.ParseInt(cursor, 10, 64)
	var oldest int64
	if err = s.Store.Pool.QueryRow(r.Context(), "SELECT COALESCE(min(revision),0) FROM events WHERE room_id=$1", room).Scan(&oldest); err != nil {
		terminal("system_unavailable")
		return
	}
	reset := cursor != "" && (parseErr != nil || last < 0 || last > rev || last < oldest-1 || rev-last > 256)
	if reset {
		if !send("reset", rev, model.NewResult("event_snapshot_reset", "control", randomID(), nil)) {
			return
		}
	} else if last > 0 {
		rows, e := s.Store.Pool.Query(r.Context(), "SELECT revision,kind FROM events WHERE room_id=$1 AND revision>$2 AND revision<=$3 ORDER BY revision LIMIT 256", room, last, rev)
		if e != nil {
			terminal("system_unavailable")
			return
		}
		for rows.Next() {
			var revision int64
			var kind string
			if rows.Scan(&revision, &kind) != nil {
				rows.Close()
				terminal("system_unavailable")
				return
			}
			if !send("change", revision, map[string]any{"room_id": room, "revision": revision, "kind": kind}) {
				rows.Close()
				return
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			terminal("system_unavailable")
			return
		}
	}
	if !send("snapshot", rev, snapshot) {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			return
		case <-ticker.C:
			if _, err = s.Store.Authenticate(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); err != nil {
				code := model.Code(err)
				if code == "system_internal_error" {
					code = "system_unavailable"
				}
				terminal(code)
				return
			}
			snapshot, err = s.Store.Snapshot(r.Context(), room, id)
			if err != nil {
				terminal(model.Code(err))
				return
			}
			if snapshot.Self.State != "active" {
				terminal(snapshot.Self.Reason)
				return
			}
			if !send("snapshot", snapshot.Self.Revision, snapshot) {
				return
			}
		}
	}
}
