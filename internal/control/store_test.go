package control

import (
	"context"
	"testing"
)

func TestV2RefusesV1AndNonemptySchema(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	_, err := s.Pool.Exec(ctx, "UPDATE schema_version SET version=1")
	must(t, err)
	if err = s.Migrate(ctx); err == nil {
		t.Fatal("V1 accepted")
	}
	var version int
	must(t, s.Pool.QueryRow(ctx, "SELECT version FROM schema_version").Scan(&version))
	if version != 1 {
		t.Fatal("changed rejected schema")
	}
	_, err = s.Pool.Exec(ctx, "DROP TABLE schema_version")
	must(t, err)
	if err = s.Migrate(ctx); err == nil {
		t.Fatal("nonempty foreign schema accepted")
	}
	var nodesRemain bool
	must(t, s.Pool.QueryRow(ctx, "SELECT to_regclass('nodes') IS NOT NULL").Scan(&nodesRemain))
	if !nodesRemain {
		t.Fatal("rejected database was modified")
	}
}
