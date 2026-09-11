package control

import (
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Server) adminRoom(w http.ResponseWriter, r *http.Request, _ string) {
	out, err := s.Store.adminRoomSnapshot(r.Context(), r.PathValue("room"))
	s.result(w, out, err)
}

func (s *Server) adminRoomAction(r *http.Request, tx pgx.Tx, actor string, raw []byte) (any, error) {
	id := r.PathValue("room")
	var in struct {
		ExpectedRevision int64  `json:"expected_revision"`
		Action           string `json:"action"`
		DeviceID         string `json:"device_id"`
	}
	if err := decodeBytes(raw, &in); err != nil {
		return nil, err
	}
	room, err := readRoom(r.Context(), tx, id)
	if err != nil {
		return nil, err
	}
	if room.Closed && in.Action == "close" {
		return model.NewResult("operation_noop", "control", "", room), nil
	}
	if in.ExpectedRevision != room.Revision {
		return nil, model.RevisionError(in.ExpectedRevision, room.Revision)
	}
	return s.Store.adminRoomAction(r.Context(), tx, actor, id, in.Action, in.DeviceID)
}
