package control

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func readRoom(ctx context.Context, tx pgx.Tx, id string) (model.Room, error) {
	var r model.Room
	err := tx.QueryRow(ctx, "SELECT r.id,r.name,r.owner_user_id,r.game,r.revision,r.capacity,r.expires_at,r.closed,g.name FROM rooms r JOIN games g ON g.id=r.game WHERE r.id=$1", id).Scan(&r.ID, &r.Name, &r.OwnerUserID, &r.Game, &r.Revision, &r.Capacity, &r.ExpiresAt, &r.Closed, &r.GameName)
	return r, noRows(err)
}

func (s *Store) OwnedRooms(ctx context.Context, device string) ([]model.Room, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, "SELECT id FROM rooms WHERE owner_user_id=(SELECT user_id FROM user_devices WHERE device_id=$1) AND NOT closed AND expires_at>now() ORDER BY expires_at DESC,id LIMIT 501", device)
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
	if err = owner(ctx, tx, roomID, device); err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, "SELECT now()").Scan(&out.ServerTime); err != nil {
		return out, err
	}
	if !out.Room.ExpiresAt.After(out.ServerTime) {
		return out, model.Failure("room_expired")
	}
	if out.Room.Closed {
		return out, model.Failure("room_closed")
	}
	out.Permissions = model.Permissions{Manage: true, Join: true}
	self, e := memberSelf(ctx, tx, roomID, device)
	if e != nil && !model.IsCode(e, "resource_not_found") {
		return out, e
	}
	out.Permissions.Leave = self.State == "active"
	out.Permissions.Join = !out.Permissions.Leave
	if out.Invitation, err = inviteInfo(ctx, tx, roomID); err != nil {
		return out, err
	}
	if out.Game, err = readGame(ctx, tx, out.Room.Game); err == nil {
		out.Members, err = readRoomMembers(ctx, tx, roomID)
	}
	return out, err
}
func (s *Store) available(ctx context.Context, tx pgx.Tx, device string) error {
	rows, err := tx.Query(ctx, `SELECT m.room_id,m.device_id FROM members m JOIN rooms r ON r.id=m.room_id JOIN user_devices d ON d.device_id=m.device_id WHERE m.user_id=(SELECT user_id FROM user_devices WHERE device_id=$1) AND m.active AND (r.closed OR r.expires_at<=now() OR d.revoked OR d.expires_at<=now())`, device)
	if err != nil {
		return err
	}
	type member struct{ room, device string }
	old, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (member, error) {
		var m member
		e := row.Scan(&m.room, &m.device)
		return m, e
	})
	if err != nil {
		return err
	}
	for _, m := range old {
		if err = revoke(ctx, tx, m.room, m.device); err != nil {
			return err
		}
		if err = bump(ctx, tx, m.room, "member_expired"); err != nil {
			return err
		}
	}
	var room, other string
	err = tx.QueryRow(ctx, `SELECT room_id,device_id FROM members WHERE user_id=(SELECT user_id FROM user_devices WHERE device_id=$1) AND active`, device).Scan(&room, &other)
	if err == nil {
		code := "account_in_use"
		if other == device {
			code = "room_already_joined"
		}
		r, e := readRoom(ctx, tx, room)
		if e != nil {
			return e
		}
		return &model.BusinessError{Code: code, Details: model.Details{RoomID: room, DeviceID: other, ActualRevision: r.Revision}}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var node bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM node_bindings WHERE device_id=$1)`, device).Scan(&node); err != nil {
		return err
	}
	if node {
		return model.Failure("auth_identity_scope_conflict")
	}
	return nil
}

func (s *Store) addMember(ctx context.Context, tx pgx.Tx, room, device string) error {
	if err := s.available(ctx, tx, device); err != nil {
		return err
	}
	r, err := readRoom(ctx, tx, room)
	if err != nil {
		return err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, "SELECT now()").Scan(&now); err != nil {
		return err
	}
	if !r.ExpiresAt.After(now) {
		return model.Failure("room_expired")
	}
	if r.Closed {
		return model.Failure("room_closed")
	}
	var banned bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM room_bans WHERE room_id=$1 AND user_id=(SELECT user_id FROM user_devices WHERE device_id=$2))`, room, device).Scan(&banned); err != nil {
		return err
	}
	if banned {
		return model.Failure("room_banned")
	}
	g, err := readGame(ctx, tx, r.Game)
	if err != nil {
		return err
	}
	if !g.Enabled {
		return model.Failure("game_disabled")
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM members WHERE room_id=$1 AND active", room).Scan(&count); err != nil {
		return err
	}
	if count >= r.Capacity {
		return &model.BusinessError{Code: "room_full", Details: model.Details{Capacity: r.Capacity, MemberCount: count}}
	}
	ip, err := s.allocate(ctx, tx, "member:"+device)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO members(room_id,device_id,user_id,ip) VALUES($1,$2,(SELECT user_id FROM user_devices WHERE device_id=$2),$3) ON CONFLICT(room_id,device_id) DO UPDATE SET ip=EXCLUDED.ip,active=true,mac='',last_seen=now()`, room, device, ip)
	return err
}

func invitation(ctx context.Context, tx pgx.Tx, room string) (model.Invitation, error) {
	out := model.Invitation{Code: randomID()}
	if err := tx.QueryRow(ctx, `SELECT LEAST(now()+interval '30 minutes',expires_at) FROM rooms WHERE id=$1`, room).Scan(&out.ExpiresAt); err != nil {
		return out, err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM invitations WHERE room_id=$1", room); err != nil {
		return out, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO invitations(code_hash,room_id,expires_at) VALUES($1,$2,$3)", hash(out.Code), room, out.ExpiresAt); err != nil {
		return out, err
	}
	if err := bump(ctx, tx, room, "invite_changed"); err != nil {
		return out, err
	}
	if err := tx.QueryRow(ctx, "SELECT revision FROM rooms WHERE id=$1", room).Scan(&out.Revision); err != nil {
		return out, err
	}
	err := adminEvent(ctx, tx, "player", "invite.issued", room, map[string]any{"room_id": room, "room_revision": out.Revision, "expires_at": out.ExpiresAt})
	return out, err
}

func (s *Store) createRoom(ctx context.Context, tx pgx.Tx, device string, in model.RoomRequest) (model.RoomResult, error) {
	var out model.RoomResult
	in.Name = strings.TrimSpace(in.Name)
	if !model.ValidLabel(in.Name, 120) {
		return out, model.Validation("name", "invalid_format")
	}
	g, err := readGame(ctx, tx, in.Game)
	if err != nil {
		return out, err
	}
	if !g.Enabled {
		return out, model.Failure("game_disabled")
	}
	if in.ExpectedGameRevision != g.Revision {
		return out, model.RevisionError(in.ExpectedGameRevision, g.Revision)
	}
	id := randomID()
	_, err = tx.Exec(ctx, "INSERT INTO rooms(id,name,owner_user_id,game,capacity,expires_at) VALUES($1,$2,(SELECT user_id FROM user_devices WHERE device_id=$3),$4,$5,now()+interval '24 hours')", id, in.Name, device, in.Game, model.RoomCapacity)
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
func (s *Store) joinRoom(ctx context.Context, tx pgx.Tx, device string, in model.JoinRequest) (any, error) {
	var out model.RoomResult
	var room string
	err := tx.QueryRow(ctx, "SELECT room_id FROM invitations WHERE code_hash=$1 AND expires_at>now()", hash(in.Code)).Scan(&room)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, model.Failure("invite_unusable")
		}
		return out, err
	}
	// An already active member may retry joining without consuming another address.
	active := activeMember(ctx, tx, room, device) == nil
	if !active {
		if err = s.addMember(ctx, tx, room, device); err != nil {
			return out, err
		}
		if err = bump(ctx, tx, room, "joined"); err != nil {
			return out, err
		}
	}
	out.Room, err = readRoom(ctx, tx, room)
	if err == nil && active {
		return model.NewResult("operation_noop", "control", "", out), nil
	}
	return out, err
}
func (s *Store) heartbeat(ctx context.Context, tx pgx.Tx, room, device string, in model.HeartbeatRequest) (any, error) {
	if err := activeMember(ctx, tx, room, device); err != nil {
		return nil, err
	}
	if in.LANVersion != model.LANVersion {
		return nil, model.Failure("lan_version_unsupported")
	}
	if in.MAC != "" {
		mac, err := model.ParseLANMAC(in.MAC)
		if err != nil {
			return nil, ErrInvalid
		}
		var duplicate bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM members WHERE room_id=$1 AND active AND device_id<>$2 AND mac=$3)`, room, device, mac.String()).Scan(&duplicate); err != nil {
			return nil, err
		}
		if duplicate {
			return nil, model.Failure("lan_mac_conflict")
		}
		result, err := tx.Exec(ctx, `UPDATE members SET mac=$3 WHERE room_id=$1 AND device_id=$2 AND mac<>$3`, room, device, mac.String())
		if err != nil {
			return nil, err
		}
		if result.RowsAffected() != 0 {
			if err = bump(ctx, tx, room, "lan_mac"); err != nil {
				return nil, err
			}
		}
	}
	_, err := tx.Exec(ctx, "UPDATE members SET last_seen=now() WHERE room_id=$1 AND device_id=$2", room, device)
	if err != nil {
		return nil, err
	}
	var accepted, valid time.Time
	if err = tx.QueryRow(ctx, `SELECT now(),LEAST(now()+interval '45 seconds',r.expires_at,COALESCE(d.expires_at,r.expires_at)) FROM rooms r CROSS JOIN user_devices d WHERE r.id=$1 AND d.device_id=$2`, room, device).Scan(&accepted, &valid); err != nil {
		return nil, err
	}
	return map[string]any{"accepted_at": accepted, "membership_valid_until": valid}, nil
}

func (s *Store) roomAction(ctx context.Context, tx pgx.Tx, room, device, action string, in model.MemberRequest) (any, error) {
	u, err := playerUser(ctx, tx, device)
	if err != nil {
		return nil, err
	}
	r, err := readRoom(ctx, tx, room)
	if err != nil {
		return nil, err
	}
	if action == "leave" {
		self, e := memberSelf(ctx, tx, room, device)
		if e != nil {
			return nil, e
		}
		if self.State != "active" {
			return model.NewResult("operation_noop", "control", "", nil), nil
		}
	} else {
		if r.OwnerUserID != u.ID {
			return nil, model.Failure("room_owner_required")
		}
		if action == "close" && r.Closed {
			return model.NewResult("operation_noop", "control", "", nil), nil
		}
		if err = owner(ctx, tx, room, device); err != nil {
			return nil, err
		}
		if in.ExpectedRevision != r.Revision {
			return nil, model.RevisionError(in.ExpectedRevision, r.Revision)
		}
	}
	switch action {
	case "leave":
		err = revoke(ctx, tx, room, device, "member_left")
	case "kick", "transfer":
		var targetUser string
		e := tx.QueryRow(ctx, "SELECT user_id FROM members WHERE room_id=$1 AND device_id=$2 AND active", room, in.DeviceID).Scan(&targetUser)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, model.Failure("member_not_active")
		}
		if e != nil {
			return nil, e
		}
		if targetUser == u.ID {
			return nil, model.Failure("member_target_self")
		}
		if e = activeMember(ctx, tx, room, in.DeviceID); e != nil {
			if model.EndsMembership(model.Code(e)) || model.EndsIdentity(model.Code(e)) {
				return nil, model.Failure("member_not_active")
			}
			return nil, e
		}
		if action == "kick" {
			if err = revoke(ctx, tx, room, in.DeviceID, "member_kicked"); err == nil {
				err = banMember(ctx, tx, room, in.DeviceID)
			}
		} else {
			if _, err = tx.Exec(ctx, "UPDATE rooms SET owner_user_id=$2 WHERE id=$1", room, targetUser); err == nil {
				_, err = tx.Exec(ctx, "DELETE FROM invitations WHERE room_id=$1", room)
			}
		}
	case "close":
		if err = revoke(ctx, tx, room, "", "room_closed"); err == nil {
			_, err = tx.Exec(ctx, "UPDATE rooms SET closed=true WHERE id=$1", room)
		}
		if err == nil {
			_, err = tx.Exec(ctx, "DELETE FROM invitations WHERE room_id=$1", room)
		}
	case "invite":
		return invitation(ctx, tx, room)
	case "invite/revoke":
		tag, e := tx.Exec(ctx, "DELETE FROM invitations WHERE room_id=$1", room)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() == 0 {
			return model.NewResult("operation_noop", "control", "", nil), nil
		}
	default:
		return nil, model.Failure("resource_not_found")
	}
	if err != nil {
		return nil, err
	}
	if err = bump(ctx, tx, room, action); err != nil {
		return nil, err
	}
	if action == "invite/revoke" {
		if err = adminEvent(ctx, tx, device, "invite.revoked", room, map[string]string{"room_id": room}); err != nil {
			return nil, err
		}
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
	if !infrastructure {
		if _, err = playerUser(ctx, tx, device); err != nil {
			return out, err
		}
	} else {
		if _, err = nodeForDevice(ctx, tx, device); err != nil {
			return out, err
		}
	}
	if room != "" {
		var permitted bool
		err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM rooms WHERE id=$1 AND owner_user_id=(SELECT user_id FROM user_devices WHERE device_id=$2)) OR EXISTS(SELECT 1 FROM members WHERE room_id=$1 AND device_id=$2)", room, device).Scan(&permitted)
		if err != nil {
			return out, err
		}
		if !permitted {
			return out, model.Failure("resource_not_found")
		}
		r, err := readRoom(ctx, tx, room)
		if err != nil {
			return out, err
		}

		self, e := memberSelf(ctx, tx, room, device)
		if e != nil {
			if !model.IsCode(e, "resource_not_found") {
				return out, e
			}
			self = model.MembershipSelf{RoomID: room, DeviceID: device, State: "none", Revision: r.Revision}
		}
		out.Self = self
		var user string
		if err = tx.QueryRow(ctx, "SELECT user_id FROM user_devices WHERE device_id=$1", device).Scan(&user); err != nil {
			return out, err
		}
		out.Permissions = model.Permissions{Manage: r.OwnerUserID == user && !r.Closed && r.ExpiresAt.After(out.ServerTime), Leave: self.State == "active"}
		out.Permissions.Join = out.Permissions.Manage && self.State != "active"
		if self.State != "active" {
			return out, nil
		}
		out.Room = &r
		g, e := readGame(ctx, tx, r.Game)
		if e != nil {
			return out, e
		}
		out.Game = &g
		if out.Members, err = readRoomMembers(ctx, tx, room); err != nil {
			return out, err
		}

	} else if !infrastructure {
		return out, model.Failure("resource_not_found")
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
	rows, err := tx.Query(ctx, "SELECT m.device_id,u.name,m.ip,m.last_seen,m.mac,m.user_id FROM members m JOIN users u ON u.id=m.user_id JOIN user_devices d ON d.device_id=m.device_id WHERE m.room_id=$1 AND m.active AND u.state IN ('active','disabled') AND NOT d.revoked AND (d.expires_at IS NULL OR d.expires_at>now()) ORDER BY m.device_id", room)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.Member, error) {
		var out model.Member
		err := row.Scan(&out.DeviceID, &out.Name, &out.IP, &out.LastSeen, &out.MAC, &out.UserID)
		return out, err
	})
}

func (s *Store) adminRoomAction(ctx context.Context, tx pgx.Tx, actor, room, action, device string) (any, error) {
	r, err := readRoom(ctx, tx, room)
	if err != nil {
		return nil, err
	}
	if r.Closed {
		return nil, model.Failure("room_closed")
	}
	switch action {
	case "close":
		if err = revoke(ctx, tx, room, "", "room_closed"); err == nil {
			_, err = tx.Exec(ctx, "UPDATE rooms SET closed=true WHERE id=$1", room)
		}
	case "kick":
		if err = activeMember(ctx, tx, room, device); err != nil {
			return nil, err
		}
		if err = revoke(ctx, tx, room, device, "member_kicked"); err == nil {
			err = banMember(ctx, tx, room, device)
		}
	default:
		return nil, ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	if action == "close" {
		if _, err = tx.Exec(ctx, "DELETE FROM invitations WHERE room_id=$1", room); err != nil {
			return nil, err
		}
	}
	if err = bump(ctx, tx, room, "admin-"+action); err != nil {
		return nil, err
	}
	if err = adminEvent(ctx, tx, actor, "room."+action, room, map[string]string{"device_id": device}); err != nil {
		return nil, err
	}
	return readRoom(ctx, tx, room)
}
