package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func readRoom(ctx context.Context, tx pgx.Tx, id string) (model.Room, error) {
	var r model.Room
	err := tx.QueryRow(ctx, "SELECT r.id,r.name,r.owner_id,r.game,r.revision,r.capacity,r.expires_at,r.closed,g.name FROM rooms r JOIN games g ON g.id=r.game WHERE r.id=$1", id).Scan(&r.ID, &r.Name, &r.OwnerID, &r.Game, &r.Revision, &r.Capacity, &r.ExpiresAt, &r.Closed, &r.GameName)
	return r, noRows(err)
}

func (s *Store) OwnedRooms(ctx context.Context, device string) ([]model.Room, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, "SELECT id FROM rooms WHERE owner_id=$1 AND NOT closed AND expires_at>now() ORDER BY expires_at DESC,id LIMIT 500", device)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := []model.Room{}
	for _, id := range ids {
		room, err := readRoom(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, room)
	}
	return out, nil
}

func (s *Store) RoomManagement(ctx context.Context, roomID, device string) (model.RoomManagement, error) {
	out := model.RoomManagement{Members: []model.Member{}}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if out.Room, err = readRoom(ctx, tx, roomID); err != nil {
		return out, err
	}
	if out.Room.OwnerID != device {
		return out, ErrForbidden
	}
	if err = tx.QueryRow(ctx, "SELECT now()").Scan(&out.ServerTime); err != nil {
		return out, err
	}
	if out.Room.Closed || !out.Room.ExpiresAt.After(out.ServerTime) {
		return out, ErrConflict
	}
	if out.Game, err = readGame(ctx, tx, out.Room.Game); err == nil {
		out.Members, err = readRoomMembers(ctx, tx, roomID)
	}
	return out, err
}
func (s *Store) available(ctx context.Context, tx pgx.Tx, device string) error {
	var used bool
	err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM members WHERE device_id=$1 AND active) OR EXISTS(SELECT 1 FROM node_bindings WHERE device_id=$1)", device).Scan(&used)
	if err != nil {
		return err
	}
	if used {
		return fmt.Errorf("%w: device already joined a room or enrolled as infrastructure", ErrConflict)
	}
	return nil
}
func (s *Store) addMember(ctx context.Context, tx pgx.Tx, room, device string) error {
	if err := s.available(ctx, tx, device); err != nil {
		return err
	}
	var banned bool
	err := tx.QueryRow(ctx, "SELECT banned FROM members WHERE room_id=$1 AND device_id=$2", room, device).Scan(&banned)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if banned {
		return ErrForbidden
	}
	var count, capacity int
	var open bool
	err = tx.QueryRow(ctx, "SELECT (SELECT count(*) FROM members WHERE room_id=$1 AND active),capacity,NOT closed AND expires_at>now() FROM rooms WHERE id=$1", room).Scan(&count, &capacity, &open)
	if err != nil {
		return noRows(err)
	}
	if !open {
		return ErrForbidden
	}
	if count >= capacity {
		return fmt.Errorf("%w: room full", ErrConflict)
	}
	ip, err := s.allocate(ctx, tx, "member:"+device)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO members(room_id,device_id,ip) VALUES($1,$2,$3) ON CONFLICT(room_id,device_id) DO UPDATE SET ip=EXCLUDED.ip,active=true,last_seen=now()", room, device, ip)
	if err != nil {
		return err
	}
	r, err := readRoom(ctx, tx, room)
	if err != nil {
		return err
	}
	g, err := readGame(ctx, tx, r.Game)
	if err != nil {
		return err
	}
	if !g.Enabled {
		return ErrForbidden
	}
	return nil
}
func invitation(ctx context.Context, tx pgx.Tx, room string) (model.Invitation, error) {
	out := model.Invitation{Code: randomID(), ExpiresAt: time.Now().UTC().Add(30 * time.Minute)}
	if _, err := tx.Exec(ctx, "DELETE FROM invitations WHERE room_id=$1", room); err != nil {
		return out, err
	}
	_, err := tx.Exec(ctx, "INSERT INTO invitations(code_hash,room_id,expires_at) VALUES($1,$2,$3)", hash(out.Code), room, out.ExpiresAt)
	return out, err
}
func (s *Store) createRoom(ctx context.Context, tx pgx.Tx, device string, in model.RoomRequest) (model.RoomResult, error) {
	var out model.RoomResult
	in.Name = strings.TrimSpace(in.Name)
	if !model.ValidLabel(in.Name, 120) {
		return out, ErrInvalid
	}
	g, err := readGame(ctx, tx, in.Game)
	if err != nil {
		return out, err
	}
	if !g.Enabled {
		return out, ErrForbidden
	}
	id := randomID()
	_, err = tx.Exec(ctx, "INSERT INTO rooms(id,name,owner_id,game,capacity,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '24 hours')", id, in.Name, device, in.Game, model.RoomCapacity)
	if err != nil {
		return out, err
	}
	if err = s.addMember(ctx, tx, id, device); err != nil {
		return out, err
	}
	inv, err := invitation(ctx, tx, id)
	if err != nil {
		return out, err
	}
	if err = bump(ctx, tx, id, "created"); err != nil {
		return out, err
	}
	out.Room, err = readRoom(ctx, tx, id)
	out.Invitation = &inv
	return out, err
}
func (s *Store) joinRoom(ctx context.Context, tx pgx.Tx, device string, in model.JoinRequest) (model.RoomResult, error) {
	var out model.RoomResult
	var room string
	err := tx.QueryRow(ctx, "SELECT room_id FROM invitations WHERE code_hash=$1 AND expires_at>now()", hash(in.Code)).Scan(&room)
	if err != nil {
		return out, ErrForbidden
	}
	// An already active member may retry joining without consuming another address.
	if err = activeMember(ctx, tx, room, device); err != nil {
		if err = s.addMember(ctx, tx, room, device); err != nil {
			return out, err
		}
		if err = bump(ctx, tx, room, "joined"); err != nil {
			return out, err
		}
	}
	out.Room, err = readRoom(ctx, tx, room)
	return out, err
}
func (s *Store) heartbeat(ctx context.Context, tx pgx.Tx, room, device string) (any, error) {
	if err := activeMember(ctx, tx, room, device); err != nil {
		return nil, err
	}
	_, err := tx.Exec(ctx, "UPDATE members SET last_seen=now() WHERE room_id=$1 AND device_id=$2", room, device)
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, err
}

func (s *Store) roomAction(ctx context.Context, tx pgx.Tx, room, device, action string, in model.MemberRequest) (any, error) {
	switch action {
	case "leave":
		if err := activeMember(ctx, tx, room, device); err != nil {
			return nil, err
		}
		if err := revoke(ctx, tx, room, device); err != nil {
			return nil, err
		}
	case "kick":
		if err := owner(ctx, tx, room, device); err != nil {
			return nil, err
		}
		if in.DeviceID == device {
			return nil, ErrInvalid
		}
		if err := activeMember(ctx, tx, room, in.DeviceID); err != nil {
			return nil, err
		}
		if err := revoke(ctx, tx, room, in.DeviceID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, "UPDATE members SET banned=true WHERE room_id=$1 AND device_id=$2", room, in.DeviceID); err != nil {
			return nil, err
		}
	case "transfer":
		if err := owner(ctx, tx, room, device); err != nil {
			return nil, err
		}
		if err := activeMember(ctx, tx, room, in.DeviceID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, "UPDATE rooms SET owner_id=$2 WHERE id=$1", room, in.DeviceID); err != nil {
			return nil, err
		}
	case "close":
		if err := owner(ctx, tx, room, device); err != nil {
			return nil, err
		}
		if err := revoke(ctx, tx, room, ""); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, "UPDATE rooms SET closed=true WHERE id=$1", room); err != nil {
			return nil, err
		}
	case "invite":
		if err := owner(ctx, tx, room, device); err != nil {
			return nil, err
		}
		return invitation(ctx, tx, room)
	default:
		return nil, ErrNotFound
	}
	if err := bump(ctx, tx, room, action); err != nil {
		return nil, err
	}
	return readRoom(ctx, tx, room)
}

func (s *Store) Snapshot(ctx context.Context, room, device string) (model.Snapshot, error) {
	out := model.Snapshot{Members: []model.Member{}, Nodes: []model.Node{}, Blocklist: []string{}}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, "SELECT now()").Scan(&out.ServerTime); err != nil {
		return out, err
	}
	var infrastructure bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM node_bindings WHERE device_id=$1)", device).Scan(&infrastructure); err != nil {
		return out, err
	}
	if room != "" {
		var permitted bool
		err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM rooms WHERE id=$1 AND owner_id=$2) OR EXISTS(SELECT 1 FROM members WHERE room_id=$1 AND device_id=$2)", room, device).Scan(&permitted)
		if err != nil {
			return out, err
		}
		if !permitted {
			return out, ErrForbidden
		}
		r, err := readRoom(ctx, tx, room)
		if err != nil {
			return out, err
		}
		out.Room = &r
		g, err := readGame(ctx, tx, r.Game)
		if err != nil {
			return out, err
		}
		out.Game = &g
		// Former members get only the tombstone needed to stop their own runtime.
		if activeMember(ctx, tx, room, device) == nil && !r.Closed {
			if out.Members, err = readRoomMembers(ctx, tx, room); err != nil {
				return out, err
			}
		}
	} else if !infrastructure {
		return out, ErrForbidden
	}
	out.Nodes, err = readNodes(ctx, tx, true)
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, "SELECT fingerprint FROM certificates WHERE revoked AND expires_at>now() AND ($1='' OR room_id=$1 OR room_id='') ORDER BY fingerprint", room)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var f string
		if err = rows.Scan(&f); err != nil {
			rows.Close()
			return out, err
		}
		out.Blocklist = append(out.Blocklist, f)
	}
	err = rows.Err()
	rows.Close()
	return out, err
}

func readRoomMembers(ctx context.Context, tx pgx.Tx, room string) ([]model.Member, error) {
	rows, err := tx.Query(ctx, "SELECT m.device_id,d.name,m.ip,m.last_seen,m.mac FROM members m JOIN devices d ON d.id=m.device_id WHERE m.room_id=$1 AND m.active ORDER BY m.device_id", room)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.Member, error) {
		var out model.Member
		err := row.Scan(&out.DeviceID, &out.Name, &out.IP, &out.LastSeen, &out.MAC)
		return out, err
	})
}

func (s *Store) adminRoomAction(ctx context.Context, tx pgx.Tx, actor, room, action, device string) (any, error) {
	r, err := readRoom(ctx, tx, room)
	if err != nil {
		return nil, err
	}
	if r.Closed {
		return nil, ErrConflict
	}
	switch action {
	case "close":
		if err = revoke(ctx, tx, room, ""); err == nil {
			_, err = tx.Exec(ctx, "UPDATE rooms SET closed=true WHERE id=$1", room)
		}
	case "kick":
		if err = activeMember(ctx, tx, room, device); err != nil {
			return nil, err
		}
		if err = revoke(ctx, tx, room, device); err == nil {
			_, err = tx.Exec(ctx, "UPDATE members SET banned=true WHERE room_id=$1 AND device_id=$2", room, device)
		}
	default:
		return nil, ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	if err = bump(ctx, tx, room, "admin-"+action); err != nil {
		return nil, err
	}
	if err = adminEvent(ctx, tx, actor, "room."+action, room, map[string]string{"device_id": device}); err != nil {
		return nil, err
	}
	return readRoom(ctx, tx, room)
}
