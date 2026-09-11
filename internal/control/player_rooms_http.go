package control

import (
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Server) playerOwnedRooms(w http.ResponseWriter, r *http.Request, id string) {
	out, err := s.Store.OwnedRooms(r.Context(), id)
	page := model.RoomPage{Rooms: out, Truncated: len(out) > 500}
	if page.Truncated {
		page.Rooms = out[:500]
	}
	s.result(w, page, err)
}

func (s *Server) playerRoomManagement(w http.ResponseWriter, r *http.Request, id string) {
	out, err := s.Store.RoomManagement(r.Context(), r.PathValue("room"), id)
	s.result(w, out, err)
}

func (s *Server) playerSnapshot(w http.ResponseWriter, r *http.Request, id string) {
	out, err := s.Store.Snapshot(r.Context(), r.PathValue("room"), id)
	s.result(w, out, err)
}

func (s *Server) playerCreateRoom(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	var in model.RoomRequest
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return s.Store.createRoom(r.Context(), tx, id, in)
}

func (s *Server) playerJoinRoom(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	var in model.JoinRequest
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return s.Store.joinRoom(r.Context(), tx, id, in)
}

func (s *Server) playerLease(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	var in model.LeaseRequest
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return s.Store.issueLease(r.Context(), tx, s.CA, r.PathValue("room"), id, in)
}

func (s *Server) playerHeartbeat(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	var in model.HeartbeatRequest
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return s.Store.heartbeat(r.Context(), tx, r.PathValue("room"), id, in)
}

func (s *Server) playerInvite(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	return s.playerMemberAction(r, tx, id, b, "invite")
}

func (s *Server) playerKick(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	return s.playerMemberAction(r, tx, id, b, "kick")
}

func (s *Server) playerTransfer(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	return s.playerMemberAction(r, tx, id, b, "transfer")
}

func (s *Server) playerLeave(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	return s.playerMemberAction(r, tx, id, b, "leave")
}

func (s *Server) playerClose(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	return s.playerMemberAction(r, tx, id, b, "close")
}

func (s *Server) playerMemberAction(r *http.Request, tx pgx.Tx, id string, b []byte, action string) (any, error) {
	var in model.MemberRequest
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return s.Store.roomAction(r.Context(), tx, r.PathValue("room"), id, action, in)
}
