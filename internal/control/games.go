package control

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

const gameColumns = `id,name,summary,source_url,ports,enabled,revision,
 EXISTS(SELECT 1 FROM game_images WHERE game_id=games.id AND kind='cover'),
 EXISTS(SELECT 1 FROM game_images WHERE game_id=games.id AND kind='background')`

func scanGame(row pgx.Row) (model.Game, error) {
	var g model.Game
	var cover, background bool
	err := row.Scan(&g.ID, &g.Name, &g.Summary, &g.SourceURL, &g.Ports, &g.Enabled, &g.Revision, &cover, &background)
	if cover {
		g.CoverURL = "/v2/games/" + g.ID + "/images/cover"
	}
	if background {
		g.BackgroundURL = "/v2/games/" + g.ID + "/images/background"
	}
	return g, noRows(err)
}

func readGame(ctx context.Context, tx pgx.Tx, id string) (model.Game, error) {
	return scanGame(tx.QueryRow(ctx, "SELECT "+gameColumns+" FROM games WHERE id=$1", id))
}

func readGames(ctx context.Context, tx pgx.Tx, enabledOnly bool) ([]model.Game, error) {
	rows, err := tx.Query(ctx, "SELECT "+gameColumns+" FROM games WHERE NOT $1 OR enabled ORDER BY (id='custom'),name,id", enabledOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Game{}
	for rows.Next() {
		g, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func expandGamePorts(ports []model.GamePort) ([]model.EndpointRequest, error) {
	out := []model.EndpointRequest{}
	seen := map[model.EndpointRequest]bool{}
	for _, p := range ports {
		end := p.PortEnd
		if end == 0 {
			end = p.Port
		}
		if (p.Protocol != "tcp" && p.Protocol != "udp") || p.Port == 0 || end < p.Port ||
			(p.Port <= model.ProbePort && end >= model.ProbePort) || (p.Description != "" && !model.ValidLabel(p.Description, 120)) || int(end)-int(p.Port)+1+len(out) > 32 {
			return nil, fmt.Errorf("%w: 端口须为 TCP/UDP 1–65535，排除 4243，范围展开后最多 32 个且不可重复", ErrInvalid)
		}
		for n := int(p.Port); n <= int(end); n++ {
			e := model.EndpointRequest{Protocol: p.Protocol, Port: uint16(n)}
			if seen[e] {
				return nil, fmt.Errorf("%w: 端口重复", ErrInvalid)
			}
			seen[e] = true
			out = append(out, e)
		}
	}
	return out, nil
}

func gameAllowsEndpoint(g model.Game, e model.EndpointRequest) bool {
	if !g.Enabled {
		return false
	}
	if g.ID == "custom" {
		return true
	}
	ports, err := expandGamePorts(g.Ports)
	if err != nil {
		return false
	}
	for _, p := range ports {
		if p.Protocol == e.Protocol && p.Port == e.Port {
			return true
		}
	}
	return false
}

// Called inside the same transaction as membership/heartbeat/configuration.
// Only configured ports are renewed here; custom ports expire
// unless the client continues to register them.
func syncGamePorts(ctx context.Context, tx pgx.Tx, room, device string, g model.Game) error {
	if !g.Enabled || g.ID == "custom" {
		return nil
	}
	ports, err := expandGamePorts(g.Ports)
	if err != nil {
		return err
	}
	changed := false
	for _, p := range ports {
		result, err := tx.Exec(ctx, `UPDATE endpoints SET expires_at=(SELECT last_seen+interval '45 seconds' FROM members WHERE room_id=$1 AND device_id=$2)
 WHERE room_id=$1 AND device_id=$2 AND protocol=$3 AND port=$4 AND expires_at>now()`, room, device, p.Protocol, p.Port)
		if err != nil {
			return err
		}
		if result.RowsAffected() > 0 {
			continue
		}
		if _, err = tx.Exec(ctx, `INSERT INTO endpoints(id,room_id,device_id,protocol,port,expires_at)
 SELECT $1,$2,$3,$4,$5,last_seen+interval '45 seconds' FROM members WHERE room_id=$2 AND device_id=$3
 ON CONFLICT(room_id,device_id,protocol,port) DO UPDATE SET expires_at=EXCLUDED.expires_at`, randomID(), room, device, p.Protocol, p.Port); err != nil {
			return err
		}
		changed = true
	}
	if changed {
		return bump(ctx, tx, room, "game_ports")
	}
	return nil
}

func (s *Store) updateGame(ctx context.Context, tx pgx.Tx, actor, id string, in model.GameUpdateRequest) (model.Game, error) {
	g, err := readGame(ctx, tx, id)
	if err != nil {
		return g, err
	}
	if id == "custom" {
		return g, ErrForbidden
	}
	if g.Revision != in.Revision {
		return g, ErrConflict
	}
	in.Name = strings.TrimSpace(in.Name)
	if !model.ValidLabel(in.Name, 200) {
		return g, ErrInvalid
	}
	if _, err = expandGamePorts(in.Ports); err != nil {
		return g, err
	}
	if in.Enabled && len(in.Ports) == 0 {
		return g, fmt.Errorf("%w: 启用前请配置游戏端口", ErrInvalid)
	}
	if in.Ports == nil {
		in.Ports = []model.GamePort{}
	}
	rulesChanged := g.Enabled != in.Enabled || !slices.Equal(g.Ports, in.Ports)
	_, err = tx.Exec(ctx, "UPDATE games SET name=$2,ports=$3,enabled=$4,revision=revision+1 WHERE id=$1", id, in.Name, in.Ports, in.Enabled)
	if err != nil {
		return g, err
	}
	g.Name, g.Ports, g.Enabled, g.Revision = in.Name, in.Ports, in.Enabled, g.Revision+1
	rows, err := tx.Query(ctx, "SELECT id FROM rooms WHERE game=$1 AND NOT closed AND expires_at>now()", id)
	if err != nil {
		return g, err
	}
	var rooms []string
	for rows.Next() {
		var room string
		if err = rows.Scan(&room); err != nil {
			rows.Close()
			return g, err
		}
		rooms = append(rooms, room)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return g, err
	}
	for _, room := range rooms {
		if rulesChanged {
			// Rebuild fixed permissions atomically, including expired rows.
			if _, err = tx.Exec(ctx, "DELETE FROM endpoints WHERE room_id=$1", room); err != nil {
				return g, err
			}
			members, err := readRoomMembers(ctx, tx, room)
			if err != nil {
				return g, err
			}
			for _, m := range members {
				// Do not extend an offline member's authorization on admin edits.
				var online bool
				if err = tx.QueryRow(ctx, "SELECT last_seen>now()-interval '45 seconds' FROM members WHERE room_id=$1 AND device_id=$2", room, m.DeviceID).Scan(&online); err != nil {
					return g, err
				}
				if online {
					if err = syncGamePorts(ctx, tx, room, m.DeviceID, g); err != nil {
						return g, err
					}
				}
			}
		}
		if err = bump(ctx, tx, room, "game_updated"); err != nil {
			return g, err
		}
	}
	if err = adminEvent(ctx, tx, actor, "game.updated", id, map[string]any{"revision": g.Revision, "enabled": g.Enabled}); err != nil {
		return g, err
	}
	return g, nil
}
