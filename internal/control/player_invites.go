package control

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func inviteInfo(ctx context.Context, tx pgx.Tx, room string) (model.InviteInfo, error) {
	var out model.InviteInfo
	var expires time.Time
	err := tx.QueryRow(ctx, "SELECT expires_at FROM invitations WHERE room_id=$1 AND expires_at>now()", room).Scan(&expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Active = true
	out.ExpiresAt = &expires
	err = tx.QueryRow(ctx, `SELECT (detail->>'room_revision')::bigint FROM admin_events WHERE kind='invite.issued' AND target=$1 ORDER BY id DESC LIMIT 1`, room).Scan(&out.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	return out, err
}

func (s *Server) playerInviteInfo(w http.ResponseWriter, r *http.Request, id string) {
	tx, err := s.Store.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if err = owner(r.Context(), tx, r.PathValue("room"), id); err != nil {
		s.fail(w, err)
		return
	}
	out, err := inviteInfo(r.Context(), tx, r.PathValue("room"))
	s.result(w, out, err)
}

func (s *Server) playerOwnerJoin(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	var in model.MemberRequest
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	ctx, room := r.Context(), r.PathValue("room")
	if err := owner(ctx, tx, room, id); err != nil {
		return nil, err
	}
	current, err := readRoom(ctx, tx, room)
	if err != nil {
		return nil, err
	}
	if activeMember(ctx, tx, room, id) == nil {
		return model.NewResult("operation_noop", "control", "", model.RoomResult{Room: current}), nil
	}
	if current.Revision != in.ExpectedRevision {
		return nil, model.RevisionError(in.ExpectedRevision, current.Revision)
	}
	if err = s.Store.addMember(ctx, tx, room, id); err != nil {
		return nil, err
	}
	if err = bump(ctx, tx, room, "joined"); err != nil {
		return nil, err
	}
	current, err = readRoom(ctx, tx, room)
	return model.RoomResult{Room: current}, err
}
