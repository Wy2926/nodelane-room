package control

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
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
	must(t, s.InitializeSchema(ctx))
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
	must(t, a.RegisterGuest(context.Background()))
	return a
}
func create(t *testing.T, a *client.API) model.RoomResult {
	t.Helper()
	var r model.RoomResult
	must(t, a.Call(context.Background(), "POST", "/v2/rooms", model.RoomRequest{ExpectedGameRevision: 1, Name: "Test room", Game: "custom"}, &r))
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

func contractRequest(method, path string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, path, body)
	r.Header.Set(model.ContractHeader, model.Contract)
	return r
}
func responseData(t *testing.T, b []byte) []byte {
	t.Helper()
	var result model.Result
	must(t, json.Unmarshal(b, &result))
	if result.Contract != model.Contract || result.RequestID == "" {
		t.Fatal("missing response contract")
	}
	if model.HTTPStatus(result.Code) >= 400 {
		return b
	}
	return result.Data
}
func roomRevision(t *testing.T, s *Store, room string) int64 {
	t.Helper()
	var revision int64
	must(t, s.Pool.QueryRow(context.Background(), "SELECT revision FROM rooms WHERE id=$1", room).Scan(&revision))
	return revision
}
func receiptData(t *testing.T, b []byte) []byte {
	t.Helper()
	var receipt model.Receipt
	must(t, json.Unmarshal(b, &receipt))
	return receipt.Result.Data
}
