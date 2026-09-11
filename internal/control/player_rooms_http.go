package control

import (
	"fmt"
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

func (s *Server) playerHeartbeat(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	var in model.HeartbeatRequest
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	ctx, room := r.Context(), r.PathValue("room")
	if err := activeMember(ctx, tx, room, id); err != nil {
		return nil, err
	}
	if in.LANVersion != model.LANVersion {
		return nil, fmt.Errorf("%w: 此房间需要支持 Ethernet LAN v1 的客户端", ErrConflict)
	}
	if in.MAC != "" {
		mac, err := model.ParseLANMAC(in.MAC)
		if err != nil || in.LANVersion != model.LANVersion {
			return nil, ErrInvalid
		}
		var duplicate bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM members WHERE room_id=$1 AND active AND device_id<>$2 AND mac=$3)`, room, id, mac.String()).Scan(&duplicate); err != nil {
			return nil, err
		}
		if duplicate {
			return nil, fmt.Errorf("%w: 房间内虚拟网卡 MAC 重复", ErrConflict)
		}
		result, err := tx.Exec(ctx, `UPDATE members SET mac=$3 WHERE room_id=$1 AND device_id=$2 AND mac<>$3`, room, id, mac.String())
		if err != nil {
			return nil, err
		}
		if result.RowsAffected() != 0 {
			if err = bump(ctx, tx, room, "lan_mac"); err != nil {
				return nil, err
			}
		}
	}
	return s.Store.heartbeat(r.Context(), tx, r.PathValue("room"), id)
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
