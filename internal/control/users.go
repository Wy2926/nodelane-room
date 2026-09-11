package control

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func playerUser(ctx context.Context, tx pgx.Tx, device string) (model.User, error) {
	var u model.User
	var revoked, expired bool
	err := tx.QueryRow(ctx, `SELECT u.id,u.name,u.kind,u.state,u.created_at,d.revoked,d.expires_at IS NOT NULL AND d.expires_at<=now() FROM users u JOIN user_devices d ON d.user_id=u.id WHERE d.device_id=$1`, device).Scan(&u.ID, &u.Name, &u.Kind, &u.State, &u.CreatedAt, &revoked, &expired)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, model.Failure("auth_device_unregistered")
	}
	if err != nil {
		return u, err
	}
	if u.State == "deleted" {
		return u, model.Failure("account_deleted")
	}
	if revoked {
		return u, model.Failure("auth_device_revoked")
	}
	if expired {
		return u, model.Failure("auth_device_expired")
	}
	return u, nil
}

func roomCreationPermission(ctx context.Context, tx pgx.Tx, device string, user model.User) (model.RoomCreationPermission, error) {
	if user.State == "disabled" {
		return model.RoomCreationPermission{Reason: "account_disabled"}, nil
	}
	required, err := updateRequired(ctx, tx, device)
	if err != nil {
		return model.RoomCreationPermission{}, err
	}
	if required {
		return model.RoomCreationPermission{Reason: "client_update_required"}, nil
	}
	return model.RoomCreationPermission{Allowed: true}, nil
}

func (s *Store) Account(ctx context.Context, device string) (model.User, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return model.User{}, err
	}
	defer tx.Rollback(ctx)
	return playerUser(ctx, tx, device)
}

func validPlayerSession(ctx context.Context, tx pgx.Tx, device, tokenHash string) error {
	if _, err := playerUser(ctx, tx, device); err != nil {
		return err
	}
	var valid bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE token_hash=$1 AND device_id=$2 AND scope='player' AND expires_at>now())`, tokenHash, device).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrUnauthorized
	}
	return nil
}

func issuePlayerSession(ctx context.Context, tx pgx.Tx, device string) (model.Session, error) {
	u, err := playerUser(ctx, tx, device)
	if err != nil {
		return model.Session{}, err
	}
	out := model.Session{Token: randomID() + randomID(), DeviceID: device, ExpiresAt: time.Now().UTC().Add(time.Hour), User: &u}
	_, err = tx.Exec(ctx, `INSERT INTO sessions(token_hash,device_id,scope,expires_at) VALUES($1,$2,'player',$3)`, hash(out.Token), device, out.ExpiresAt)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE user_devices SET last_seen=now() WHERE device_id=$1`, device)
	}
	return out, err
}

// Revocation and room events commit with the caller's state change. Peers receive
// both the removed LAN member and the old certificate fingerprints.
func revokeUserConnections(ctx context.Context, tx pgx.Tx, user, device string, closeOwned bool) error {
	rows, err := tx.Query(ctx, `SELECT DISTINCT room_id FROM members WHERE user_id=$1 AND active AND ($2='' OR device_id=$2) UNION SELECT id FROM rooms WHERE owner_user_id=$1 AND $3 AND NOT closed`, user, device, closeOwned)
	if err != nil {
		return err
	}
	rooms, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, room := range rooms {
		var owned bool
		if err = tx.QueryRow(ctx, `SELECT owner_user_id=$2 AND $3 FROM rooms WHERE id=$1`, room, user, closeOwned).Scan(&owned); err != nil {
			return err
		}
		if owned {
			if err = revoke(ctx, tx, room, ""); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE rooms SET closed=true WHERE id=$1`, room); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `DELETE FROM invitations WHERE room_id=$1`, room); err != nil {
				return err
			}
		} else {
			rows, e := tx.Query(ctx, `SELECT device_id FROM members WHERE room_id=$1 AND user_id=$2 AND active AND ($3='' OR device_id=$3)`, room, user, device)
			if e != nil {
				return e
			}
			devices, e := pgx.CollectRows(rows, pgx.RowTo[string])
			if e != nil {
				return e
			}
			for _, d := range devices {
				if err = revoke(ctx, tx, room, d); err != nil {
					return err
				}
			}
		}
		if err = bump(ctx, tx, room, "user_revoked"); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE scope='player' AND device_id IN (SELECT device_id FROM user_devices WHERE user_id=$1 AND ($2='' OR device_id=$2))`, user, device)
	return err
}

func banMember(ctx context.Context, tx pgx.Tx, room, device string) error {
	_, err := tx.Exec(ctx, `INSERT INTO room_bans(room_id,user_id) SELECT room_id,user_id FROM members WHERE room_id=$1 AND device_id=$2 ON CONFLICT DO NOTHING`, room, device)
	return err
}

func revokePlayerDevice(ctx context.Context, tx pgx.Tx, id string, target string) (any, error) {
	u, err := playerUser(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if u.Kind != "registered" {
		return nil, model.Failure("account_guest_logout_forbidden")
	}
	if target == id {
		return nil, model.Failure("member_target_self")
	}
	var revoked bool
	err = tx.QueryRow(ctx, "SELECT revoked FROM user_devices WHERE device_id=$1 AND user_id=$2", target, u.ID).Scan(&revoked)
	if err != nil {
		return nil, noRows(err)
	}
	if revoked {
		return model.NewResult("operation_noop", "control", "", nil), nil
	}
	if _, err = tx.Exec(ctx, "UPDATE user_devices SET revoked=true WHERE device_id=$1", target); err == nil {
		err = revokeUserConnections(ctx, tx, u.ID, target, false)
	}
	return map[string]bool{"revoked": err == nil}, err
}

func logoutPlayer(ctx context.Context, tx pgx.Tx, id string) (any, error) {
	u, err := playerUser(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if u.Kind == "guest" {
		return nil, model.Failure("account_guest_logout_forbidden")
	}
	if _, err = tx.Exec(ctx, `UPDATE user_devices SET revoked=true WHERE device_id=$1`, id); err == nil {
		err = revokeUserConnections(ctx, tx, u.ID, id, false)
	}
	return map[string]bool{"ok": err == nil}, err
}

func takeoverPlayer(ctx context.Context, tx pgx.Tx, id string, in model.TakeoverRequest) (any, error) {
	u, err := playerUser(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	var actualRoom, actualDevice string
	var actualRevision int64
	err = tx.QueryRow(ctx, `SELECT m.room_id,m.device_id,r.revision FROM members m JOIN rooms r ON r.id=m.room_id WHERE m.user_id=$1 AND m.active AND m.device_id<>$2`, u.ID, id).Scan(&actualRoom, &actualDevice, &actualRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.Failure("request_state_stale")
	}
	if err != nil {
		return nil, err
	}
	if in.RoomID != actualRoom || in.DeviceID != actualDevice || in.ExpectedRevision != actualRevision {
		return nil, model.RevisionError(in.ExpectedRevision, actualRevision)
	}
	rows, err := tx.Query(ctx, `SELECT room_id,device_id FROM members WHERE user_id=$1 AND active AND device_id<>$2`, u.ID, id)
	if err != nil {
		return nil, err
	}
	type member struct{ room, device string }
	all, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (member, error) {
		var m member
		e := row.Scan(&m.room, &m.device)
		return m, e
	})
	if err != nil {
		return nil, err
	}
	for _, m := range all {
		if err = revoke(ctx, tx, m.room, m.device, "member_taken_over"); err != nil {
			return nil, err
		}
		if err = bump(ctx, tx, m.room, "device_takeover"); err != nil {
			return nil, err
		}
	}
	return map[string]bool{"ok": true}, nil
}

func (s *Store) listUsers(ctx context.Context, q, after string) (model.UserPage, error) {
	if len(q) > 80 || len(after) > 64 {
		return model.UserPage{}, ErrInvalid
	}
	rows, err := s.Pool.Query(ctx, `SELECT id,name,kind,state,created_at FROM users WHERE id>$1 AND ($2='' OR strpos(lower(name),lower($2))>0 OR id=$2) ORDER BY id LIMIT 51`, after, q)
	if err != nil {
		return model.UserPage{}, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.User, error) {
		var u model.User
		e := row.Scan(&u.ID, &u.Name, &u.Kind, &u.State, &u.CreatedAt)
		return u, e
	})
	out := model.UserPage{Users: list}
	if len(list) > 50 {
		out.Users = list[:50]
		out.Next = list[49].ID
	}
	return out, err
}

func (s *Store) userDetail(ctx context.Context, id string) (model.UserDetail, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return model.UserDetail{}, err
	}
	defer tx.Rollback(ctx)
	out := model.UserDetail{Devices: []model.UserDevice{}, Rooms: []model.Room{}, Sessions: []model.UserSession{}}
	err = tx.QueryRow(ctx, `SELECT id,name,kind,state,created_at FROM users WHERE id=$1`, id).Scan(&out.User.ID, &out.User.Name, &out.User.Kind, &out.User.State, &out.User.CreatedAt)
	if err != nil {
		return model.UserDetail{}, noRows(err)
	}
	rows, err := tx.Query(ctx, `SELECT ud.device_id,d.name,ud.revoked,ud.last_seen,ud.expires_at,s.report,s.reported_at FROM user_devices ud JOIN devices d ON d.id=ud.device_id LEFT JOIN device_software s ON s.device_id=d.id WHERE user_id=$1 ORDER BY ud.device_id`, out.User.ID)
	if err != nil {
		return model.UserDetail{}, err
	}
	out.Devices, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.UserDevice, error) {
		var d model.UserDevice
		var b []byte
		var at *time.Time
		e := row.Scan(&d.DeviceID, &d.Name, &d.Revoked, &d.LastSeen, &d.ExpiresAt, &b, &at)
		if e == nil && at != nil {
			d.Software = &model.DeviceSoftware{ReportedAt: *at}
			e = json.Unmarshal(b, &d.Software.ClientReport)
		}
		return d, e
	})
	if err != nil {
		return model.UserDetail{}, err
	}
	rows, err = tx.Query(ctx, `SELECT DISTINCT id FROM rooms WHERE owner_user_id=$1 OR id IN (SELECT room_id FROM members WHERE user_id=$1) ORDER BY id LIMIT 100`, out.User.ID)
	if err != nil {
		return model.UserDetail{}, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return model.UserDetail{}, err
	}
	for _, id := range ids {
		room, e := readRoom(ctx, tx, id)
		if e != nil {
			return model.UserDetail{}, e
		}
		out.Rooms = append(out.Rooms, room)
	}
	rows, err = tx.Query(ctx, `SELECT s.device_id,s.expires_at FROM sessions s JOIN user_devices d ON d.device_id=s.device_id WHERE d.user_id=$1 AND s.scope='player' AND s.expires_at>now() ORDER BY s.expires_at DESC LIMIT 100`, out.User.ID)
	if err != nil {
		return model.UserDetail{}, err
	}
	out.Sessions, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.UserSession, error) {
		var x model.UserSession
		e := row.Scan(&x.DeviceID, &x.ExpiresAt)
		return x, e
	})
	return out, err
}

func userAction(ctx context.Context, tx pgx.Tx, actor, id string, in model.UserAction) (any, error) {
	if !model.ValidLabel(in.Reason, 500) {
		return nil, ErrInvalid
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM users WHERE id=$1`, id).Scan(&state); err != nil {
		return nil, noRows(err)
	}
	if state == "deleted" {
		return nil, model.Failure("admin_user_deleted")
	}
	switch in.Action {
	case "enable", "disable":
		next := "active"
		if in.Action == "disable" {
			next = "disabled"
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET state=$2 WHERE id=$1`, id, next); err != nil {
			return nil, err
		}
	case "delete", "logout", "revoke-device":
		device := ""
		if in.Action == "revoke-device" {
			device = in.DeviceID
			var valid bool
			if device == "" {
				return nil, ErrInvalid
			}
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_devices WHERE user_id=$1 AND device_id=$2)`, id, device).Scan(&valid); err != nil {
				return nil, err
			}
			if !valid {
				return nil, ErrNotFound
			}
		}
		if in.Action == "delete" {
			if _, err := tx.Exec(ctx, `UPDATE users SET state='deleted' WHERE id=$1`, id); err != nil {
				return nil, err
			}
		}
		if in.Action == "logout" || in.Action == "revoke-device" {
			if _, err := tx.Exec(ctx, `UPDATE user_devices SET revoked=true WHERE user_id=$1 AND ($2='' OR device_id=$2)`, id, device); err != nil {
				return nil, err
			}
		}
		if err := revokeUserConnections(ctx, tx, id, device, in.Action == "delete"); err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalid
	}
	err := adminEvent(ctx, tx, actor, "user."+in.Action, id, in)
	return map[string]bool{"ok": err == nil}, err
}
