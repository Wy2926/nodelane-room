package control

import (
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Server) playerTelemetry(w http.ResponseWriter, r *http.Request, id string) {
	s.acceptTelemetry(w, r, id, false)
}
func (s *Server) nodeTelemetry(w http.ResponseWriter, r *http.Request, id string) {
	s.acceptTelemetry(w, r, id, true)
}
func (s *Server) adminTelemetry(w http.ResponseWriter, r *http.Request, _ string) {
	writeJSON(w, 200, s.telemetry.snapshot(time.Now().UTC(), s.GeoIP))
}

func (s *Server) acceptTelemetry(w http.ResponseWriter, r *http.Request, id string, node bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	b, err := io.ReadAll(r.Body)
	var sample model.NetworkSample
	if err != nil || decodeBytes(b, &sample) != nil || !validSample(sample, time.Now()) {
		s.fail(w, ErrInvalid)
		return
	}
	ctx := r.Context()
	tx, err := s.Store.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback(ctx)
	identity := model.TelemetrySeries{DeviceID: id}
	if node {
		n, e := nodeForDevice(ctx, tx, id)
		if e != nil || n.State != "active" && n.State != "draining" {
			s.fail(w, ErrForbidden)
			return
		}
		identity.NodeID = n.ID
		if sample.Traffic != nil && sample.Traffic.Scope != "nebula_udp" {
			s.fail(w, ErrInvalid)
			return
		}
	} else {
		identity.RoomID = r.PathValue("room")
		if err = activeMember(ctx, tx, identity.RoomID, id); err != nil {
			s.fail(w, err)
			return
		}
		room, e := readRoom(ctx, tx, identity.RoomID)
		if e != nil || room.Closed || !room.ExpiresAt.After(time.Now()) {
			s.fail(w, ErrForbidden)
			return
		}
		if sample.Traffic != nil && sample.Traffic.Scope != "overlay" {
			s.fail(w, ErrInvalid)
			return
		}
	}
	ids := make([]string, 0, len(sample.Peers))
	for _, p := range sample.Peers {
		ids = append(ids, p.DeviceID)
	}
	// Scope every observed identity/IP using current shared authorization. Delayed
	// revoked peers are discarded, not allowed to overwrite another room's data.
	rows, err := tx.Query(ctx, `SELECT m.device_id,m.ip FROM members m JOIN rooms r ON r.id=m.room_id WHERE m.active AND NOT r.closed AND r.expires_at>now() AND m.device_id=ANY($1) AND ($2 OR m.room_id=$3)
	UNION ALL SELECT b.device_id,b.ip FROM node_bindings b JOIN nodes n ON n.id=b.node_id WHERE b.revoked_at IS NULL AND n.state IN ('active','draining') AND b.device_id=ANY($1)`, ids, node, identity.RoomID)
	if err != nil {
		s.fail(w, err)
		return
	}
	allowed := map[string]string{}
	for rows.Next() {
		var device, ip string
		if err = rows.Scan(&device, &ip); err != nil {
			break
		}
		allowed[device] = ip
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		s.fail(w, err)
		return
	}
	peers := make([]model.LinkSample, 0, len(sample.Peers))
	for _, p := range sample.Peers {
		if p.DeviceID != id && allowed[p.DeviceID] == p.IP {
			peers = append(peers, p)
		}
	}
	sample.Peers = peers
	if err = tx.Commit(ctx); err == nil {
		err = s.telemetry.put(identity, sample, time.Now().UTC())
	}
	s.result(w, map[string]bool{"ok": err == nil}, err)
}
