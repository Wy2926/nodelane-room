package control

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

type AdminEvent struct {
	ID        int64           `json:"id"`
	Actor     string          `json:"actor"`
	Kind      string          `json:"kind"`
	Target    string          `json:"target"`
	Detail    json.RawMessage `json:"detail"`
	CreatedAt time.Time       `json:"created_at"`
}
type AdminSnapshot struct {
	Revision     int64                 `json:"revision"`
	ServerTime   time.Time             `json:"server_time"`
	Version      string                `json:"version"`
	Network      string                `json:"network"`
	DeploymentID string                `json:"deployment_id"`
	CAExpiresAt  time.Time             `json:"ca_expires_at"`
	Nodes        []model.Node          `json:"nodes"`
	Rooms        []model.Room          `json:"rooms"`
	Operations   []model.NodeOperation `json:"operations"`
	Events       []AdminEvent          `json:"events"`
}

type AdminRoomSnapshot struct {
	Room       model.Room       `json:"room"`
	Members    []model.Member   `json:"members"`
	Endpoints  []model.Endpoint `json:"endpoints"`
	ServerTime time.Time        `json:"server_time"`
}

func (s *Store) adminRoomSnapshot(ctx context.Context, room string) (AdminRoomSnapshot, error) {
	var out AdminRoomSnapshot
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, "SELECT now()").Scan(&out.ServerTime); err != nil {
		return out, err
	}
	if out.Room, err = readRoom(ctx, tx, room); err != nil {
		return out, err
	}
	if out.Members, err = readRoomMembers(ctx, tx, room); err != nil {
		return out, err
	}
	out.Endpoints, err = readRoomEndpoints(ctx, tx, room)
	return out, err
}

func (s *Server) adminSnapshot(ctx context.Context) (AdminSnapshot, error) {
	out := AdminSnapshot{Version: model.Version, Network: s.Store.Network.String(), ServerTime: time.Now().UTC(), CAExpiresAt: s.CA.Certificate.NotAfter(), Rooms: []model.Room{}, Operations: []model.NodeOperation{}, Events: []AdminEvent{}}
	tx, err := s.Store.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, "SELECT value FROM settings WHERE key='deployment_id'").Scan(&out.DeploymentID); err != nil {
		return out, err
	}
	out.Nodes, err = readNodes(ctx, tx, false)
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, "SELECT id,name,owner_id,game,revision,capacity,expires_at,closed FROM rooms ORDER BY expires_at DESC LIMIT 500")
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var r model.Room
		if err = rows.Scan(&r.ID, &r.Name, &r.OwnerID, &r.Game, &r.Revision, &r.Capacity, &r.ExpiresAt, &r.Closed); err != nil {
			rows.Close()
			return out, err
		}
		out.Rooms = append(out.Rooms, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, "SELECT id,node_id,generation,revision,action,state,error,created_at,expires_at FROM node_operations ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var op model.NodeOperation
		if err = rows.Scan(&op.ID, &op.NodeID, &op.Generation, &op.Revision, &op.Action, &op.State, &op.Error, &op.CreatedAt, &op.ExpiresAt); err != nil {
			rows.Close()
			return out, err
		}
		if (op.State == "pending" || op.State == "running") && op.ExpiresAt.Before(out.ServerTime) {
			op.State = "expired"
		}
		out.Operations = append(out.Operations, op)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, "SELECT id,actor,kind,target,detail,created_at FROM admin_events ORDER BY id DESC LIMIT 200")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var e AdminEvent
		if err = rows.Scan(&e.ID, &e.Actor, &e.Kind, &e.Target, &e.Detail, &e.CreatedAt); err != nil {
			return out, err
		}
		out.Events = append(out.Events, e)
		if e.ID > out.Revision {
			out.Revision = e.ID
		}
	}
	return out, rows.Err()
}
