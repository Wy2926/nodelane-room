package control

import (
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Server) playerOwnedRooms(w http.ResponseWriter, r *http.Request, id string) {
	out, err := s.Store.OwnedRooms(r.Context(), id)
	s.result(w, out, err)
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

func (s *Server) playerEndpoint(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	var in model.EndpointRequest
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return s.Store.endpoint(r.Context(), tx, r.PathValue("room"), id, in)
}

func (s *Server) playerHeartbeat(r *http.Request, tx pgx.Tx, id string, _ []byte) (any, error) {
	return s.Store.heartbeat(r.Context(), tx, r.PathValue("room"), id)
}

func (s *Server) playerDeleteEndpoint(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	var in model.EndpointRequest
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	ctx, room := r.Context(), r.PathValue("room")
	if err := activeMember(ctx, tx, room, id); err != nil {
		return nil, err
	}
	rm, err := readRoom(ctx, tx, room)
	if err != nil {
		return nil, err
	}
	if rm.Game != "custom" {
		return nil, ErrForbidden
	}
	if _, err = expandGamePorts([]model.GamePort{{Protocol: in.Protocol, Port: in.Port}}); err != nil {
		return nil, err
	}
	result, err := tx.Exec(ctx, "DELETE FROM endpoints WHERE room_id=$1 AND device_id=$2 AND protocol=$3 AND port=$4", room, id, in.Protocol, in.Port)
	if err == nil && result.RowsAffected() > 0 {
		err = bump(ctx, tx, room, "endpoint_removed")
	}
	return map[string]bool{"ok": err == nil}, err
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
