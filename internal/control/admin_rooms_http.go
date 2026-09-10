package control

import (
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (s *Server) adminRoom(w http.ResponseWriter, r *http.Request, _ string) {
	out, err := s.Store.adminRoomSnapshot(r.Context(), r.PathValue("room"))
	s.result(w, out, err)
}

func (s *Server) adminRoomAction(r *http.Request, tx pgx.Tx, actor string, raw []byte) (any, error) {
	id := r.PathValue("room")
	var in struct {
		Action   string `json:"action"`
		DeviceID string `json:"device_id"`
	}
	if err := decodeBytes(raw, &in); err != nil {
		return nil, err
	}
	return s.Store.adminRoomAction(r.Context(), tx, actor, id, in.Action, in.DeviceID)
}
