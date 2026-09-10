package control

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/netip"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
)

func database(t *testing.T) (*Store, *pki.Authority) {
	t.Helper()
	url := os.Getenv("NODELANE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set NODELANE_TEST_DATABASE_URL for real PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	must(t, err)
	schema := "test_" + randomID()
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	must(t, err)
	cfg, err := pgxpool.ParseConfig(url)
	must(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	must(t, err)
	s := &Store{Pool: pool, Network: netip.MustParsePrefix(model.DefaultPool)}
	must(t, s.Migrate(ctx))
	ca, err := pki.Generate(s.Network)
	must(t, err)
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
	})
	return s, ca
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func apiServer(t *testing.T, s *Store, ca *pki.Authority) *httptest.Server {
	t.Helper()
	h := (&Server{Store: s, CA: ca, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	return server
}
func user(t *testing.T, server *httptest.Server, name string) *client.API {
	t.Helper()
	i, err := device.NewIdentity(server.URL, name)
	must(t, err)
	a := client.NewAPI(i)
	a.HTTP = server.Client()
	must(t, a.Authenticate(context.Background()))
	return a
}
func create(t *testing.T, a *client.API) model.RoomResult {
	t.Helper()
	var r model.RoomResult
	must(t, a.Call(context.Background(), "POST", "/v2/rooms", model.RoomRequest{Name: "Test room", Game: "minecraft-java"}, &r))
	return r
}
func join(t *testing.T, a *client.API, r model.RoomResult) {
	t.Helper()
	must(t, a.Call(context.Background(), "POST", "/v2/rooms/join", model.JoinRequest{Code: r.Invitation.Code}, nil))
}
func lease(t *testing.T, a *client.API, room string) model.Lease {
	t.Helper()
	_, pub, err := pki.TunnelKey()
	must(t, err)
	var l model.Lease
	must(t, a.Call(context.Background(), "POST", "/v2/rooms/"+room+"/lease", model.LeaseRequest{PublicKey: pub}, &l))
	return l
}
func statusError(t *testing.T, err error, status int) {
	t.Helper()
	var e *client.APIError
	if !errors.As(err, &e) || e.Status != status {
		t.Fatalf("want API status %d, got %v", status, err)
	}
}
