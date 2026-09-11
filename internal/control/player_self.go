package control

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func memberSelf(ctx context.Context, tx pgx.Tx, room, device string) (model.MembershipSelf, error) {
	out := model.MembershipSelf{RoomID: room, DeviceID: device, State: "ended", Reason: "member_revoked"}
	var active, closed, expired bool
	var valid time.Time
	err := tx.QueryRow(ctx, `SELECT m.active,r.closed,r.expires_at<=now(),r.revision,LEAST(m.last_seen+interval '45 seconds',r.expires_at,COALESCE(d.expires_at,r.expires_at)) FROM members m JOIN rooms r ON r.id=m.room_id JOIN user_devices d ON d.device_id=m.device_id WHERE m.room_id=$1 AND m.device_id=$2`, room, device).Scan(&active, &closed, &expired, &out.Revision, &valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, model.Failure("resource_not_found")
	}
	if err != nil {
		return out, err
	}
	if expired {
		out.Reason = "room_expired"
		return out, nil
	}
	if closed {
		out.Reason = "room_closed"
		return out, nil
	}
	if active {
		out.State, out.Reason, out.ValidUntil = "active", "", &valid
		return out, nil
	}
	var reason string
	err = tx.QueryRow(ctx, `SELECT detail->>'reason_code' FROM admin_events WHERE kind='member.ended' AND target=$1 AND detail->>'device_id'=$2 ORDER BY id DESC LIMIT 1`, room, device).Scan(&reason)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if model.EndsMembership(reason) {
		out.Reason = reason
	}
	return out, nil
}

func (s *Store) AccountStatus(ctx context.Context, device string) (model.AccountStatus, error) {
	var out model.AccountStatus
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if out.User, err = playerUser(ctx, tx, device); err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, `SELECT d.device_id,v.name,d.revoked,d.last_seen,d.expires_at FROM user_devices d JOIN devices v ON v.id=d.device_id WHERE d.device_id=$1`, device).Scan(&out.Device.DeviceID, &out.Device.Name, &out.Device.Revoked, &out.Device.LastSeen, &out.Device.ExpiresAt); err != nil {
		return out, err
	}
	out.Membership.State = "none"
	var lastRoom string
	err = tx.QueryRow(ctx, "SELECT room_id FROM members WHERE device_id=$1 ORDER BY last_seen DESC,room_id LIMIT 1", device).Scan(&lastRoom)
	if err == nil {
		out.Membership, err = memberSelf(ctx, tx, lastRoom, device)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	var room, occupiedDevice string
	err = tx.QueryRow(ctx, `SELECT room_id,device_id FROM members WHERE user_id=$1 AND active`, out.User.ID).Scan(&room, &occupiedDevice)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	self, err := memberSelf(ctx, tx, room, occupiedDevice)
	if occupiedDevice == device {
		out.Membership = self
	} else if self.State == "active" {
		out.Occupancy = &self
	}
	return out, err
}

func (s *Store) userDevices(ctx context.Context, device string) ([]model.UserDevice, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	u, err := playerUser(ctx, tx, device)
	if err != nil {
		return nil, err
	}
	if u.Kind != "registered" {
		return nil, model.Failure("account_link_not_allowed")
	}
	rows, err := tx.Query(ctx, `SELECT d.device_id,v.name,d.revoked,d.last_seen,d.expires_at FROM user_devices d JOIN devices v ON v.id=d.device_id WHERE d.user_id=$1 ORDER BY d.device_id`, u.ID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.UserDevice, error) {
		var d model.UserDevice
		e := row.Scan(&d.DeviceID, &d.Name, &d.Revoked, &d.LastSeen, &d.ExpiresAt)
		return d, e
	})
	return out, err
}
