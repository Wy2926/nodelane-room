package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) playerEvents(w http.ResponseWriter, r *http.Request, id string) {
	room := r.PathValue("room")
	snapshot, err := s.Store.Snapshot(r.Context(), room, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, errors.New("streaming unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	if r.Method == http.MethodHead {
		return
	}
	s.streams.Add(1)
	defer s.streams.Add(-1)
	last, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	send := func(kind string, rev int64, payload any) bool {
		b, e := json.Marshal(payload)
		if e != nil {
			return false
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
		_, e = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", rev, kind, b)
		if e != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	// Snapshot establishes a consistent starting point even if the event history
	// was pruned. Old events are replayed only as invalidation notifications.
	if last > 0 {
		rows, e := s.Store.Pool.Query(r.Context(), "SELECT revision,kind FROM events WHERE room_id=$1 AND revision>$2 AND revision<=$3 ORDER BY revision LIMIT 256", room, last, snapshot.Room.Revision)
		if e == nil {
			for rows.Next() {
				var rev int64
				var kind string
				if rows.Scan(&rev, &kind) != nil {
					break
				}
				if !send("change", rev, map[string]string{"kind": kind}) {
					rows.Close()
					return
				}
			}
			rows.Close()
		}
	}
	if !send("snapshot", snapshot.Room.Revision, snapshot) {
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
				return
			}
			snapshot, err = s.Store.Snapshot(r.Context(), room, id)
			if err != nil {
				return
			}
			if !send("snapshot", snapshot.Room.Revision, snapshot) {
				return
			}
		}
	}
}
