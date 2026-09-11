package control

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func TestSchemaRefusesOlderVersionsAndNonemptyDatabase(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	_, err := s.Pool.Exec(ctx, "UPDATE schema_version SET version=3")
	must(t, err)
	if err = s.InitializeSchema(ctx); err == nil {
		t.Fatal("older schema accepted")
	}
	var version int
	must(t, s.Pool.QueryRow(ctx, "SELECT version FROM schema_version").Scan(&version))
	if version != 3 {
		t.Fatal("changed rejected schema")
	}
	_, err = s.Pool.Exec(ctx, "DROP TABLE schema_version")
	must(t, err)
	if err = s.InitializeSchema(ctx); err == nil {
		t.Fatal("nonempty foreign schema accepted")
	}
	var nodesRemain bool
	must(t, s.Pool.QueryRow(ctx, "SELECT to_regclass('nodes') IS NOT NULL").Scan(&nodesRemain))
	if !nodesRemain {
		t.Fatal("rejected database was modified")
	}
}

func TestOpeningCurrentSchemaPreservesConfiguration(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		_, err := s.updateGame(ctx, tx, "admin", "minecraft-java", model.GameUpdateRequest{Name: "Configured game", Network: model.GameNetwork{Version: 1}, Enabled: false, Revision: 1, Ports: []model.GamePort{}})
		return err
	}))
	var deploymentID string
	must(t, s.Pool.QueryRow(ctx, "SELECT value FROM settings WHERE key='deployment_id'").Scan(&deploymentID))
	must(t, s.InitializeSchema(ctx))
	var name, reopenedID string
	var enabled bool
	var revision int64
	must(t, s.Pool.QueryRow(ctx, "SELECT name,enabled,revision FROM games WHERE id='minecraft-java'").Scan(&name, &enabled, &revision))
	must(t, s.Pool.QueryRow(ctx, "SELECT value FROM settings WHERE key='deployment_id'").Scan(&reopenedID))
	if name != "Configured game" || enabled || revision != 2 || reopenedID != deploymentID {
		t.Fatal("opening current schema reset shared configuration")
	}
}
