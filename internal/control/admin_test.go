package control

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

type adminClient struct {
	t      *testing.T
	server *httptest.Server
	cookie *http.Cookie
	csrf   string
}

func (a *adminClient) request(method, path string, body any, origin, csrf bool) (int, []byte) {
	a.t.Helper()
	return a.requestKey(method, path, body, origin, csrf, randomID())
}

func (a *adminClient) requestKey(method, path string, body any, origin, csrf bool, key string) (int, []byte) {
	a.t.Helper()
	b, e := json.Marshal(body)
	must(a.t, e)
	req, e := http.NewRequest(method, a.server.URL+"/v2/admin"+path, bytes.NewReader(b))
	must(a.t, e)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	if origin {
		req.Header.Set("Origin", a.server.URL)
	}
	if csrf {
		req.Header.Set("X-CSRF-Token", a.csrf)
	}
	if a.cookie != nil {
		req.AddCookie(a.cookie)
	}
	resp, e := a.server.Client().Do(req)
	must(a.t, e)
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == adminCookie {
			a.cookie = c
		}
	}
	data, e := io.ReadAll(resp.Body)
	must(a.t, e)
	return resp.StatusCode, data
}
func newAdmin(t *testing.T) (*Store, *adminClient) {
	s, ca := database(t)
	srv := httptest.NewTLSServer((&Server{Store: s, CA: ca, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler())
	t.Cleanup(srv.Close)
	a := &adminClient{t: t, server: srv}
	encoded, err := passwordHash("correct test password")
	must(t, err)
	must(t, s.initialize(context.Background(), SetupRequest{Username: "owner", DatabaseURL: "postgres://test:test@localhost/test", PublicURL: srv.URL, Registry: DefaultRegistry}, ca, encoded, func() error { return nil }))
	status, data := a.request("POST", "/login", map[string]string{"username": "owner", "password": "correct test password"}, true, false)
	if status != 200 {
		t.Fatalf("login: %d %s", status, data)
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	must(t, json.Unmarshal(data, &session))
	a.csrf = session.CSRF
	if a.cookie == nil || !a.cookie.Secure || !a.cookie.HttpOnly || a.cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("insecure administrator cookie")
	}
	return s, a
}

func TestAdminSessionAndCSRF(t *testing.T) {
	s, a := newAdmin(t)
	ctx := context.Background()
	status, data := a.request("GET", "/session", nil, false, false)
	var restored map[string]string
	must(t, json.Unmarshal(data, &restored))
	if status != 200 || restored["csrf"] != a.csrf {
		t.Fatal("page reload invalidates other tab's CSRF")
	}
	for _, path := range []string{"/session", "/snapshot", "/events"} {
		status, _ = a.request("HEAD", path, nil, false, false)
		if status != 200 {
			t.Fatalf("HEAD %s: %d", path, status)
		}
	}
	for _, route := range []struct{ method, path string }{
		{"POST", "/logout"}, {"POST", "/password"}, {"POST", "/nodes"},
		{"PUT", "/nodes/example"}, {"POST", "/nodes/example/key"},
		{"POST", "/nodes/example/actions"}, {"POST", "/rooms/example/actions"},
	} {
		for _, flags := range [][2]bool{{false, true}, {true, false}} {
			status, _ := a.request(route.method, route.path, map[string]string{}, flags[0], flags[1])
			if status != 403 {
				t.Fatalf("csrf/origin for %s %s: %d", route.method, route.path, status)
			}
		}
	}
	status, data = a.request("POST", "/nodes", model.NodeConfig{Name: "n", Region: "d", Address: "n:4242", Relay: true}, true, true)
	if status != 200 {
		t.Fatalf("create: %s", data)
	}
	must(t, s.ResetAdminPassword(ctx, "replacement password"))
	status, _ = a.request("GET", "/snapshot", nil, true, true)
	if status != 401 {
		t.Fatalf("reset did not revoke: %d", status)
	}
}

func TestAdminNodeRoutesAndIdempotency(t *testing.T) {
	s, a := newAdmin(t)
	ctx := context.Background()
	config := model.NodeConfig{Name: "relay", Region: "test", Address: "relay:4242", Relay: true}
	key := randomID()
	status, data := a.requestKey("POST", "/nodes", config, true, true, key)
	if status != 200 {
		t.Fatalf("create: %d", status)
	}
	var node, replay model.Node
	must(t, json.Unmarshal(data, &node))
	status, data = a.requestKey("POST", "/nodes", config, true, true, key)
	must(t, json.Unmarshal(data, &replay))
	if status != 200 || replay.ID != node.ID {
		t.Fatal("node creation replay changed the result")
	}
	path := "/nodes/" + node.ID
	// An identical body and key on another endpoint must not replay creation.
	status, _ = a.requestKey("PUT", path, config, true, true, key)
	if status != 409 {
		t.Fatalf("cross-endpoint replay: %d", status)
	}
	var failed int
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM admin_events WHERE kind='admin.operation_failed' AND target=$1", "/v2/admin"+path).Scan(&failed))
	if failed != 1 {
		t.Fatal("failed write was not audited once")
	}
	config.Address = "relay:4545"
	status, data = a.request("PUT", path, map[string]any{"config": config, "revision": node.Revision}, true, true)
	must(t, json.Unmarshal(data, &replay))
	if status != 200 || replay.Address != config.Address || replay.Revision != node.Revision+1 {
		t.Fatal("node update did not apply the requested configuration")
	}
	status, data = a.request("GET", path+"/compose", nil, false, false)
	if status != 200 || !bytes.Contains(data, []byte("4545")) || !bytes.Contains(data, []byte("services:")) {
		t.Fatal("compose route did not return the updated node deployment")
	}
	status, data = a.request("POST", path+"/key", nil, true, true)
	var enrollment struct {
		Key string `json:"key"`
	}
	must(t, json.Unmarshal(data, &enrollment))
	if status != 200 || len(enrollment.Key) != 64 {
		t.Fatal("node key route failed")
	}
	status, data = a.request("POST", path+"/actions", map[string]string{"action": "revoke-key"}, true, true)
	var operation model.NodeOperation
	must(t, json.Unmarshal(data, &operation))
	if status != 200 || operation.NodeID != node.ID || operation.Action != "revoke-key" {
		t.Fatal("node action route dispatched incorrectly")
	}
	var revoked bool
	must(t, s.Pool.QueryRow(ctx, "SELECT revoked FROM enrollment_keys WHERE key_hash=$1", hash(enrollment.Key)).Scan(&revoked))
	if !revoked {
		t.Fatal("node action did not revoke the enrollment key")
	}
	// A cached successful mutation remains protected by the current session.
	_, err := s.Pool.Exec(ctx, "UPDATE admin_sessions SET expires_at=now()-interval '1 second'")
	must(t, err)
	status, _ = a.requestKey("POST", "/nodes", config, true, true, key)
	if status != 401 {
		t.Fatalf("expired session replay: %d", status)
	}
}

func TestAdminPasswordRouteRevokesSessions(t *testing.T) {
	s, a := newAdmin(t)
	status, _ := a.request("POST", "/password", map[string]string{"current": "wrong password", "password": "new test password"}, true, true)
	if status != 401 {
		t.Fatalf("wrong current password: %d", status)
	}
	status, _ = a.request("POST", "/password", map[string]string{"current": "correct test password", "password": "new test password"}, true, true)
	if status != 200 {
		t.Fatalf("password change: %d", status)
	}
	var sessions, events int
	must(t, s.Pool.QueryRow(context.Background(), "SELECT (SELECT count(*) FROM admin_sessions),(SELECT count(*) FROM admin_events WHERE kind='admin.password_changed')").Scan(&sessions, &events))
	if sessions != 0 || events != 1 {
		t.Fatal("password change did not revoke sessions and record its audit event")
	}
	status, _ = a.request("GET", "/session", nil, false, false)
	if status != 401 {
		t.Fatalf("old session retained access: %d", status)
	}
	status, _ = a.request("POST", "/login", map[string]string{"username": "owner", "password": "correct test password"}, true, false)
	if status != 401 {
		t.Fatalf("old password accepted: %d", status)
	}
	status, _ = a.request("POST", "/login", map[string]string{"username": "owner", "password": "new test password"}, true, false)
	if status != 200 {
		t.Fatalf("new password rejected: %d", status)
	}
}

func TestAdminRoomSnapshotAfterOwnerLeaves(t *testing.T) {
	_, a := newAdmin(t)
	ctx := context.Background()
	owner := user(t, a.server, "owner")
	member := user(t, a.server, "member")
	room := create(t, owner)
	join(t, member, room)
	path := "/v2/rooms/" + room.Room.ID
	must(t, member.Call(ctx, "POST", path+"/endpoints", model.EndpointRequest{Protocol: "tcp", Port: 25565}, nil))
	must(t, owner.Call(ctx, "POST", path+"/leave", model.MemberRequest{}, nil))
	status, data := a.request("GET", "/rooms/"+room.Room.ID, nil, false, false)
	if status != 200 {
		t.Fatalf("admin room read: %d", status)
	}
	var detail AdminRoomSnapshot
	must(t, json.Unmarshal(data, &detail))
	if detail.Room.ID != room.Room.ID || len(detail.Members) != 1 || detail.Members[0].DeviceID != member.Identity.ID() || len(detail.Endpoints) != 1 || detail.ServerTime.IsZero() {
		t.Fatal("admin snapshot lost room details after owner left")
	}
	var fields map[string]json.RawMessage
	must(t, json.Unmarshal(data, &fields))
	for _, name := range []string{"nodes", "blocklist"} {
		if _, found := fields[name]; found {
			t.Fatalf("admin room response includes unrelated %s", name)
		}
	}
	var tombstone model.Snapshot
	must(t, owner.Call(ctx, "GET", path, nil, &tombstone))
	if len(tombstone.Members) != 0 || len(tombstone.Endpoints) != 0 {
		t.Fatal("former owner regained member visibility")
	}
	outsider := user(t, a.server, "outsider")
	statusError(t, outsider.Call(ctx, "GET", path, nil, nil), 403)
	statusError(t, outsider.Call(ctx, "GET", "/v2/admin/rooms/"+room.Room.ID, nil, nil), 401)
	status, _ = a.request("POST", "/rooms/"+room.Room.ID+"/actions", map[string]string{"action": "close"}, true, true)
	if status != 200 {
		t.Fatalf("admin close: %d", status)
	}
	status, data = a.request("GET", "/rooms/"+room.Room.ID, nil, false, false)
	must(t, json.Unmarshal(data, &detail))
	if status != 200 || !detail.Room.Closed || len(detail.Members) != 0 || len(detail.Endpoints) != 0 {
		t.Fatal("closed room retained members or endpoints")
	}
}

func TestAdminRateLogoutAndSSERecovery(t *testing.T) {
	_, a := newAdmin(t)
	req, e := http.NewRequest("GET", a.server.URL+"/v2/admin/events", nil)
	must(t, e)
	req.AddCookie(a.cookie)
	req.Header.Set("Last-Event-ID", "99999999999")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, e := a.server.Client().Do(req)
	must(t, e)
	scanner := bufio.NewScanner(resp.Body)
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			var snap AdminSnapshot
			must(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &snap))
			found = snap.Version == model.Version
			break
		}
	}
	resp.Body.Close()
	if !found {
		t.Fatal("future cursor did not restore full snapshot")
	}
	status, _ := a.request("POST", "/logout", map[string]bool{}, true, true)
	if status != 200 {
		t.Fatal("logout failed")
	}
	status, _ = a.request("GET", "/snapshot", nil, false, false)
	if status != 401 {
		t.Fatal("logout retained access")
	}
	for i := 0; i < 10; i++ {
		status, _ = a.request("POST", "/login", map[string]string{"username": "unknown", "password": "bad"}, true, false)
	}
	if status != 429 {
		t.Fatal("login not limited")
	}
}
