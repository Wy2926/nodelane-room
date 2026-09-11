package control

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

const nodeSelect = `SELECT n.id,COALESCE(b.device_id,''),n.name,n.region,COALESCE(b.ip,''),n.address,n.lighthouse,n.relay,n.notes,n.state,n.generation,n.revision,n.last_seen,n.report FROM nodes n LEFT JOIN node_bindings b ON b.node_id=n.id AND b.revoked_at IS NULL`

func scanNode(row pgx.Row) (model.Node, error) {
	var n model.Node
	err := row.Scan(&n.ID, &n.DeviceID, &n.Name, &n.Region, &n.IP, &n.Address, &n.Lighthouse, &n.Relay, &n.Notes, &n.State, &n.Generation, &n.Revision, &n.LastSeen, &n.Report)
	n.Draining = n.State == "draining" || time.Since(n.LastSeen) > 45*time.Second || n.Report.Engine != "running"
	return n, noRows(err)
}
func nodeForDevice(ctx context.Context, tx pgx.Tx, device string) (model.Node, error) {
	n, err := scanNode(tx.QueryRow(ctx, nodeSelect+` WHERE b.device_id=$1 AND b.generation=n.generation`, device))
	if errors.Is(err, ErrNotFound) {
		var state string
		var stale bool
		e := tx.QueryRow(ctx, "SELECT n.state,b.generation<>n.generation FROM node_bindings b JOIN nodes n ON n.id=b.node_id WHERE b.device_id=$1", device).Scan(&state, &stale)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return n, e
		}
		if state == "revoked" {
			return n, model.Failure("node_revoked")
		}
		if stale {
			return n, model.Failure("node_generation_stale")
		}
		return n, model.Failure("node_not_authorized")
	}
	if err != nil {
		return n, err
	}
	if n.State == "revoked" {
		return n, model.Failure("node_revoked")
	}
	return n, nil
}
func readNode(ctx context.Context, tx pgx.Tx, id string) (model.Node, error) {
	return scanNode(tx.QueryRow(ctx, nodeSelect+" WHERE n.id=$1", id))
}
func readNodes(ctx context.Context, tx pgx.Tx, authorized bool) ([]model.Node, error) {
	q := nodeSelect
	if authorized {
		q += ` WHERE n.state IN ('active','draining') AND b.device_id IS NOT NULL`
	}
	rows, err := tx.Query(ctx, q+" ORDER BY n.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Node{}
	for rows.Next() {
		n, e := scanNode(rows)
		if e != nil {
			return nil, e
		}
		if authorized && n.Report.AppliedConfig != nil {
			// Advertise only the endpoint and roles acknowledged by the current binding.
			c := n.Report.AppliedConfig
			n.Address, n.Lighthouse, n.Relay = c.Address, c.Lighthouse, c.Relay
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
func validNode(c model.NodeConfig) bool {
	host, port, e := net.SplitHostPort(c.Address)
	p, pe := strconv.Atoi(port)
	return e == nil && pe == nil && p >= 1024 && p <= 65535 && model.ValidLabel(host, 253) && !strings.ContainsAny(host, " /\\@?#") && model.ValidLabel(c.Name, 80) && model.ValidLabel(c.Region, 80) && (c.Notes == "" || model.ValidLabel(c.Notes, 1000)) && (c.Lighthouse || c.Relay)
}
func (s *Store) createNode(ctx context.Context, tx pgx.Tx, actor string, c model.NodeConfig) (model.Node, error) {
	if !validNode(c) {
		return model.Node{}, ErrInvalid
	}
	id := randomID()
	_, err := tx.Exec(ctx, "INSERT INTO nodes(id,name,region,address,lighthouse,relay,notes) VALUES($1,$2,$3,$4,$5,$6,$7)", id, c.Name, c.Region, c.Address, c.Lighthouse, c.Relay, c.Notes)
	if err != nil {
		return model.Node{}, err
	}
	if err = saveNodeConfig(ctx, tx, id, 1, c); err != nil {
		return model.Node{}, err
	}
	if err = adminEvent(ctx, tx, actor, "node.created", id, c); err != nil {
		return model.Node{}, err
	}
	return readNode(ctx, tx, id)
}
func (s *Store) IssueEnrollmentKey(ctx context.Context, nodeID, actor string) (string, error) {
	return s.issueEnrollmentKey(ctx, nodeID, actor, nil)
}
func (s *Store) issueEnrollmentKey(ctx context.Context, nodeID, actor string, check func(pgx.Tx) error) (string, error) {
	key := randomID() + randomID()
	err := s.Write(ctx, func(tx pgx.Tx) error {
		if check != nil {
			if err := check(tx); err != nil {
				return err
			}
		}
		n, err := readNode(ctx, tx, nodeID)
		if err != nil {
			return err
		}
		if n.State != "pending" {
			return model.Failure("node_state_conflict")
		}
		if _, err = tx.Exec(ctx, "UPDATE enrollment_keys SET revoked=true WHERE node_id=$1 AND consumed_by IS NULL", nodeID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO enrollment_keys(id,key_hash,node_id,generation,revision,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '30 minutes')`, randomID(), hash(key), nodeID, n.Generation, n.Revision); err != nil {
			return err
		}
		return adminEvent(ctx, tx, actor, "node.key_issued", nodeID, map[string]any{"expires_in": 1800})
	})
	if err != nil {
		return "", err
	}
	return key, nil
}
func (s *Store) completeEnrollment(ctx context.Context, tx pgx.Tx, grant, device, name string, pub []byte) error {
	var node string
	var generation, revision int64
	if err := tx.QueryRow(ctx, `UPDATE enrollment_keys SET consumed_by=$2 WHERE id=$1 AND consumed_by IS NULL AND NOT revoked AND expires_at>now() RETURNING node_id,generation,revision`, grant, device).Scan(&node, &generation, &revision); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Failure("node_enrollment_unusable")
		}
		return err
	}
	n, err := readNode(ctx, tx, node)
	if err != nil {
		return err
	}
	if n.Generation != generation || n.Revision != revision || n.State != "pending" {
		return model.Failure("node_enrollment_unusable")
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM devices WHERE id=$1)", device).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return model.Failure("node_state_conflict")
	}
	if _, err = tx.Exec(ctx, "INSERT INTO devices(id,name,public_key) VALUES($1,$2,$3)", device, name, pub); err != nil {
		return err
	}
	ip, err := s.allocate(ctx, tx, "node:"+device)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO node_bindings(device_id,node_id,generation,ip) VALUES($1,$2,$3,$4)", device, node, generation, ip); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE nodes SET state='active',last_seen=now() WHERE id=$1", node); err != nil {
		return err
	}
	return adminEvent(ctx, tx, "node:"+device, "node.enrolled", node, map[string]any{"device_id": device, "generation": generation})
}
func (s *Store) updateNode(ctx context.Context, tx pgx.Tx, actor, id string, c model.NodeConfig, revision int64) (model.Node, error) {
	if !validNode(c) {
		return model.Node{}, ErrInvalid
	}
	n, err := readNode(ctx, tx, id)
	if err != nil {
		return n, err
	}
	if n.State == "revoked" {
		return n, model.Failure("node_revoked")
	}
	if n.Revision != revision {
		return n, model.RevisionError(revision, n.Revision)
	}
	_, err = tx.Exec(ctx, `UPDATE nodes SET name=$2,region=$3,address=$4,lighthouse=$5,relay=$6,notes=$7,revision=revision+1 WHERE id=$1`, id, c.Name, c.Region, c.Address, c.Lighthouse, c.Relay, c.Notes)
	if err != nil {
		return n, err
	}
	if _, err = tx.Exec(ctx, "UPDATE enrollment_keys SET revoked=true WHERE node_id=$1 AND consumed_by IS NULL", id); err != nil {
		return n, err
	}
	if err = saveNodeConfig(ctx, tx, id, n.Revision+1, c); err != nil {
		return n, err
	}
	if err = adminEvent(ctx, tx, actor, "node.configured", id, c); err != nil {
		return n, err
	}
	return readNode(ctx, tx, id)
}
func revokeNode(ctx context.Context, tx pgx.Tx, n model.Node, permanent bool) error {
	if n.DeviceID != "" {
		if _, err := tx.Exec(ctx, "UPDATE certificates SET revoked=true WHERE device_id=$1", n.DeviceID); err != nil {
			return err
		}
		if permanent {
			if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE device_id=$1", n.DeviceID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "UPDATE node_bindings SET revoked_at=now() WHERE device_id=$1", n.DeviceID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE addresses SET release_after=GREATEST(now()+interval '11 minutes',COALESCE((SELECT max(expires_at) FROM certificates WHERE device_id=$1),now())),holder=holder||':'||$2 WHERE holder='node:'||$1`, n.DeviceID, randomID()); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *Store) nodeAction(ctx context.Context, tx pgx.Tx, actor, id, action string) (model.NodeOperation, error) {
	var op model.NodeOperation
	n, err := readNode(ctx, tx, id)
	if err != nil {
		return op, err
	}
	if n.State == "revoked" {
		return op, model.Failure("node_revoked")
	}
	next := n.State
	switch action {
	case "drain":
		if n.State != "active" {
			return op, model.Failure("node_state_conflict")
		}
		next = "draining"
	case "resume":
		if n.State != "disabled" && n.State != "draining" {
			return op, model.Failure("node_state_conflict")
		}
		next = "active"
	case "disable":
		if n.DeviceID == "" {
			return op, model.Failure("node_state_conflict")
		}
		next = "disabled"
		err = revokeNode(ctx, tx, n, false)
	case "revoke":
		next = "revoked"
		err = revokeNode(ctx, tx, n, true)
	case "replace":
		next = "pending"
		err = revokeNode(ctx, tx, n, true)
		n.Generation++
	case "restart":
		if n.DeviceID == "" || n.State == "disabled" {
			return op, model.Failure("node_state_conflict")
		}
	case "revoke-key":
	default:
		return op, ErrInvalid
	}
	if err != nil {
		return op, err
	}
	if action != "restart" {
		if _, err = tx.Exec(ctx, "UPDATE enrollment_keys SET revoked=true WHERE node_id=$1 AND consumed_by IS NULL", id); err != nil {
			return op, err
		}
	}
	n.Revision++
	if _, err = tx.Exec(ctx, "UPDATE nodes SET state=$2,generation=$3,revision=$4 WHERE id=$1", id, next, n.Generation, n.Revision); err != nil {
		return op, err
	}
	if err = saveNodeConfig(ctx, tx, id, n.Revision, n.Config()); err != nil {
		return op, err
	}
	if action == "replace" {
		if _, err = tx.Exec(ctx, "UPDATE nodes SET last_seen='epoch',report='{}' WHERE id=$1", id); err != nil {
			return op, err
		}
	}
	op = model.NodeOperation{ID: randomID(), NodeID: id, Generation: n.Generation, Revision: n.Revision, Action: action, State: "pending", CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(24 * time.Hour)}
	if action == "revoke" || action == "replace" || action == "revoke-key" {
		op.State = "succeeded"
	}
	if _, err = tx.Exec(ctx, "INSERT INTO node_operations(id,node_id,generation,revision,action,state,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)", op.ID, id, op.Generation, op.Revision, op.Action, op.State, op.ExpiresAt); err != nil {
		return op, err
	}
	return op, adminEvent(ctx, tx, actor, "node."+action, id, map[string]any{"operation_id": op.ID, "authorization_changed": true})
}
func (s *Store) SyncNode(ctx context.Context, device string, in model.NodeSyncRequest) (model.NodeSync, error) {
	var out model.NodeSync
	if len(in.Results) > 64 || len(in.Report.Probes) > 64 || len(in.Report.Error) > 2000 || !model.ValidLabel(in.Report.Version, 80) {
		return out, ErrInvalid
	}
	if in.Report.Engine != "running" && in.Report.Engine != "stopped" {
		return out, ErrInvalid
	}
	for _, p := range in.Report.Probes {
		if len(p.DeviceID) != 64 || len(p.Address) > 300 || len(p.Remote) > 300 || len(p.Mode) > 20 || p.At.After(time.Now().Add(time.Minute)) {
			return out, ErrInvalid
		}
	}
	if in.Report.Error != "" {
		in.Report.Error = safeNodeCode("failed", in.Report.Error)
	}
	err := s.Write(ctx, func(tx pgx.Tx) error {
		n, err := nodeForDevice(ctx, tx, device)
		if err != nil {
			return err
		}
		if in.Generation != n.Generation {
			return model.Failure("node_generation_stale")
		}
		if in.Report.AppliedRevision > n.Revision {
			return model.Failure("node_applied_config_invalid")
		}
		if in.Report.AppliedRevision > 0 {
			if in.Report.AppliedConfig == nil {
				return model.Failure("node_applied_config_invalid")
			}
			b, e := json.Marshal(in.Report.AppliedConfig)
			if e != nil {
				return e
			}
			var valid bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM node_configs WHERE node_id=$1 AND revision=$2 AND config=$3::jsonb)", n.ID, in.Report.AppliedRevision, b).Scan(&valid); e != nil {
				return e
			}
			if !valid {
				return model.Failure("node_applied_config_invalid")
			}
		} else if in.Report.AppliedConfig != nil {
			return model.Failure("node_applied_config_invalid")
		}

		for _, res := range in.Results {
			res.Error = safeNodeCode(res.State, res.Error)
			if res.State != "succeeded" && res.State != "failed" && res.State != "running" {
				return ErrInvalid
			}
			if len(res.Error) > 2000 {
				return ErrInvalid
			}
			tag, e := tx.Exec(ctx, `UPDATE node_operations SET state=$4,error=$5 WHERE id=$1 AND node_id=$2 AND generation=$3 AND state IN ('pending','running') AND expires_at>now() AND (state<>$4 OR error<>$5) AND ($4<>'succeeded' OR revision<=$6)`, res.ID, n.ID, n.Generation, res.State, safeNodeCode(res.State, res.Error), in.Report.AppliedRevision)
			if e != nil {
				return e
			}
			if tag.RowsAffected() > 0 {
				if e = adminEvent(ctx, tx, "node:"+device, "node.operation_result", n.ID, res); e != nil {
					return e
				}
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE node_operations SET state='expired' WHERE node_id=$1 AND state IN ('pending','running') AND (expires_at<=now() OR generation<>$2)", n.ID, n.Generation); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE node_operations SET state='superseded' WHERE node_id=$1 AND state='pending' AND revision<$2 AND action<>'restart'", n.ID, in.Report.AppliedRevision); err != nil {
			return err
		}
		persistentReport := in.Report
		persistentReport.Probes = nil
		b, err := json.Marshal(persistentReport)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE nodes SET last_seen=now(),report=$2 WHERE id=$1", n.ID, b); err != nil {
			return err
		}
		if n.Report.Engine != in.Report.Engine || n.Report.Error != in.Report.Error || n.Report.AppliedRevision != in.Report.AppliedRevision || time.Since(n.LastSeen) > 45*time.Second {
			if err = adminEvent(ctx, tx, "node:"+device, "node.status", n.ID, map[string]any{"engine": in.Report.Engine, "revision": in.Report.AppliedRevision}); err != nil {
				return err
			}
		}
		out.Node, err = readNode(ctx, tx, n.ID)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,node_id,generation,revision,action,state,error,created_at,expires_at FROM node_operations WHERE node_id=$1 AND generation=$2 AND state IN ('pending','running') AND expires_at>now() ORDER BY created_at,id LIMIT 64`, n.ID, n.Generation)
		if err != nil {
			return err
		}
		defer rows.Close()
		out.Operations = []model.NodeOperation{}
		for rows.Next() {
			var op model.NodeOperation
			if err = rows.Scan(&op.ID, &op.NodeID, &op.Generation, &op.Revision, &op.Action, &op.State, &op.Error, &op.CreatedAt, &op.ExpiresAt); err != nil {
				return err
			}
			out.Operations = append(out.Operations, op)
		}
		return rows.Err()
	})
	if err != nil {
		return out, err
	}
	out.Snapshot, err = s.Snapshot(ctx, "", device)
	return out, err
}

func saveNodeConfig(ctx context.Context, tx pgx.Tx, id string, revision int64, c model.NodeConfig) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO node_configs(node_id,revision,config) VALUES($1,$2,$3)", id, revision, b)
	return err
}

func safeNodeCode(state, code string) string {
	if state != "failed" {
		return ""
	}
	if _, ok := model.BusinessCodes[code]; ok {
		return code
	}
	return "node_operation_failed"
}
