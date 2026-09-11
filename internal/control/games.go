package control

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

const gameColumns = `id,name,summary,source_url,ports,enabled,revision,
 EXISTS(SELECT 1 FROM game_images WHERE game_id=games.id AND kind='cover'),
 EXISTS(SELECT 1 FROM game_images WHERE game_id=games.id AND kind='background'),
 network`

func scanGame(row pgx.Row) (model.Game, error) {
	var g model.Game
	var cover, background bool
	err := row.Scan(&g.ID, &g.Name, &g.Summary, &g.SourceURL, &g.Ports, &g.Enabled, &g.Revision, &cover, &background, &g.Network)
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

func (s *Store) updateGame(ctx context.Context, tx pgx.Tx, actor, id string, in model.GameUpdateRequest) (model.Game, error) {
	g, err := readGame(ctx, tx, id)
	if err != nil {
		return g, err
	}
	if g.Revision != in.Revision {
		return g, model.RevisionError(in.Revision, g.Revision)
	}
	in.Name = strings.TrimSpace(in.Name)
	if !model.ValidLabel(in.Name, 200) {
		return g, ErrInvalid
	}
	if err = model.ValidateLAN(model.Game{Network: in.Network, Ports: in.Ports}); err != nil {
		return g, model.Validation("network", "invalid_format")
	}
	if in.Enabled && len(in.Ports) == 0 && len(in.Network.EthernetTypes) == 0 {
		return g, model.Failure("game_policy_missing")
	}
	if in.Ports == nil {
		in.Ports = []model.GamePort{}
	}
	if in.Network.EthernetTypes == nil {
		in.Network.EthernetTypes = []uint16{}
	}
	_, err = tx.Exec(ctx, "UPDATE games SET name=$2,ports=$3,enabled=$4,network=$5,revision=revision+1 WHERE id=$1", id, in.Name, in.Ports, in.Enabled, in.Network)
	if err != nil {
		return g, err
	}
	g.Network = in.Network
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
		if err = bump(ctx, tx, room, "game_updated"); err != nil {
			return g, err
		}
	}
	if err = adminEvent(ctx, tx, actor, "game.updated", id, map[string]any{"revision": g.Revision, "enabled": g.Enabled}); err != nil {
		return g, err
	}
	return g, nil
}
