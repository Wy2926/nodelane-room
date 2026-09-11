package control

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

const activeUserDevice = `SELECT u.id,u.name,u.kind,u.state,u.created_at FROM users u JOIN user_devices d ON d.user_id=u.id WHERE d.device_id=$1 AND NOT d.revoked AND (d.expires_at IS NULL OR d.expires_at>now()) AND u.state='active'`

func playerUser(ctx context.Context, tx pgx.Tx, device string) (model.User, error) {
	var u model.User
	err := tx.QueryRow(ctx, activeUserDevice, device).Scan(&u.ID, &u.Name, &u.Kind, &u.State, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return u, ErrForbidden
	}
	return u, err
}

func (s *Store) Account(ctx context.Context, device string) (model.User, error) {
	var u model.User
	err := s.Pool.QueryRow(ctx, activeUserDevice, device).Scan(&u.ID, &u.Name, &u.Kind, &u.State, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return u, ErrForbidden
	}
	return u, err
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

func (s *Server) registerUsers(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/me", s.playerAuth(func(w http.ResponseWriter, r *http.Request, id string) {
		out, err := s.Store.Account(r.Context(), id)
		s.result(w, out, err)
	}))
	mux.HandleFunc("POST /v2/me/logout", s.playerMutation(func(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
		var in struct{}
		if err := decodeBytes(b, &in); err != nil {
			return nil, err
		}
		u, err := playerUser(r.Context(), tx, id)
		if err != nil {
			return nil, err
		}
		if u.Kind == "guest" {
			return nil, fmt.Errorf("%w: bind an account before signing out", ErrConflict)
		}
		if _, err = tx.Exec(r.Context(), `UPDATE user_devices SET revoked=true WHERE device_id=$1`, id); err == nil {
			err = revokeUserConnections(r.Context(), tx, u.ID, id, false)
		}
		return map[string]bool{"ok": err == nil}, err
	}))
	mux.HandleFunc("POST /v2/me/takeover", s.playerMutation(func(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
		var in struct{}
		if err := decodeBytes(b, &in); err != nil {
			return nil, err
		}
		u, err := playerUser(r.Context(), tx, id)
		if err != nil {
			return nil, err
		}
		rows, err := tx.Query(r.Context(), `SELECT room_id,device_id FROM members WHERE user_id=$1 AND active AND device_id<>$2`, u.ID, id)
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
			if err = revoke(r.Context(), tx, m.room, m.device); err != nil {
				return nil, err
			}
			if err = bump(r.Context(), tx, m.room, "device_takeover"); err != nil {
				return nil, err
			}
		}
		return map[string]bool{"ok": true}, nil
	}))
	mux.HandleFunc("GET /v2/admin/users", s.adminHandler(s.adminUsers))
	mux.HandleFunc("GET /v2/admin/users/{user}", s.adminHandler(s.adminUser))
	mux.HandleFunc("POST /v2/admin/users/{user}/actions", s.adminWrite(s.adminMutation(s.adminUserAction)))
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request, _ string) {
	q, after := strings.TrimSpace(r.URL.Query().Get("q")), r.URL.Query().Get("after")
	if len(q) > 80 || len(after) > 64 {
		s.fail(w, ErrInvalid)
		return
	}
	rows, err := s.Store.Pool.Query(r.Context(), `SELECT id,name,kind,state,created_at FROM users WHERE id>$1 AND ($2='' OR strpos(lower(name),lower($2))>0 OR id=$2) ORDER BY id LIMIT 51`, after, q)
	if err != nil {
		s.fail(w, err)
		return
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
	s.result(w, out, err)
}

func (s *Server) adminUser(w http.ResponseWriter, r *http.Request, _ string) {
	ctx := r.Context()
	tx, err := s.Store.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback(ctx)
	out := model.UserDetail{Devices: []model.UserDevice{}, Rooms: []model.Room{}, Sessions: []model.UserSession{}}
	err = tx.QueryRow(ctx, `SELECT id,name,kind,state,created_at FROM users WHERE id=$1`, r.PathValue("user")).Scan(&out.User.ID, &out.User.Name, &out.User.Kind, &out.User.State, &out.User.CreatedAt)
	if err != nil {
		s.fail(w, noRows(err))
		return
	}
	rows, err := tx.Query(ctx, `SELECT ud.device_id,d.name,ud.revoked,ud.last_seen,ud.expires_at,s.report,s.reported_at FROM user_devices ud JOIN devices d ON d.id=ud.device_id LEFT JOIN device_software s ON s.device_id=d.id WHERE user_id=$1 ORDER BY ud.device_id`, out.User.ID)
	if err != nil {
		s.fail(w, err)
		return
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
		s.fail(w, err)
		return
	}
	rows, err = tx.Query(ctx, `SELECT DISTINCT id FROM rooms WHERE owner_user_id=$1 OR id IN (SELECT room_id FROM members WHERE user_id=$1) ORDER BY id LIMIT 100`, out.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		s.fail(w, err)
		return
	}
	for _, id := range ids {
		room, e := readRoom(ctx, tx, id)
		if e != nil {
			s.fail(w, e)
			return
		}
		out.Rooms = append(out.Rooms, room)
	}
	rows, err = tx.Query(ctx, `SELECT s.device_id,s.expires_at FROM sessions s JOIN user_devices d ON d.device_id=s.device_id WHERE d.user_id=$1 AND s.scope='player' AND s.expires_at>now() ORDER BY s.expires_at DESC LIMIT 100`, out.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	out.Sessions, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.UserSession, error) {
		var x model.UserSession
		e := row.Scan(&x.DeviceID, &x.ExpiresAt)
		return x, e
	})
	s.result(w, out, err)
}

func (s *Server) adminUserAction(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
	var in model.UserAction
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	if !model.ValidLabel(in.Reason, 500) {
		return nil, ErrInvalid
	}
	ctx, id := r.Context(), r.PathValue("user")
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM users WHERE id=$1`, id).Scan(&state); err != nil {
		return nil, noRows(err)
	}
	if state == "deleted" {
		return nil, ErrConflict
	}
	switch in.Action {
	case "enable":
		if _, err := tx.Exec(ctx, `UPDATE users SET state='active' WHERE id=$1`, id); err != nil {
			return nil, err
		}
	case "disable", "delete", "logout", "revoke-device":
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
		if in.Action == "disable" || in.Action == "delete" {
			next := "disabled"
			if in.Action == "delete" {
				next = "deleted"
			}
			if _, err := tx.Exec(ctx, `UPDATE users SET state=$2 WHERE id=$1`, id, next); err != nil {
				return nil, err
			}
		}
		if in.Action == "logout" || in.Action == "revoke-device" {
			if _, err := tx.Exec(ctx, `UPDATE user_devices SET revoked=true WHERE user_id=$1 AND ($2='' OR device_id=$2)`, id, device); err != nil {
				return nil, err
			}
		}
		if err := revokeUserConnections(ctx, tx, id, device, in.Action == "disable" || in.Action == "delete"); err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalid
	}
	err := adminEvent(ctx, tx, actor, "user."+in.Action, id, in)
	return map[string]bool{"ok": err == nil}, err
}
