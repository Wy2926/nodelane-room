package control

import (
	"context"
	"crypto/ed25519"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
)

func testNode(t *testing.T, s *Store) (model.Node, string) {
	t.Helper()
	ctx := context.Background()
	var n model.Node
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		var e error
		n, e = s.createNode(ctx, tx, "test", model.NodeConfig{Name: "test node", Region: "default", Address: "127.0.0.1:4242", Lighthouse: true, Relay: true})
		return e
	}))
	key, err := s.IssueEnrollmentKey(ctx, n.ID, "test")
	must(t, err)
	return n, key
}
func nodeAPI(t *testing.T, server *httptest.Server) *client.API {
	t.Helper()
	i, err := client.NewIdentity(server.URL, "node")
	must(t, err)
	i.Node = true
	a := client.NewAPI(i)
	a.HTTP = server.Client()
	return a
}

func TestEnrollmentRejectsBeforeChallengeAndScopes(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	a := nodeAPI(t, server)
	ctx := context.Background()
	statusError(t, a.Enroll(ctx, strings.Repeat("0", 64)), 403)
	var count int
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM challenges").Scan(&count))
	if count != 0 {
		t.Fatal("invalid key created a challenge")
	}
	n, key := testNode(t, s)
	must(t, a.Enroll(ctx, key))
	statusError(t, a.Call(ctx, "POST", "/v2/rooms", model.RoomRequest{Name: "illegal", Game: "custom"}, nil), 401)
	statusError(t, a.Call(ctx, "GET", "/v2/admin/snapshot", nil, nil), 401)
	player := client.NewAPI(a.Identity)
	player.Identity.Node = false
	statusError(t, player.Authenticate(ctx), 403)
	other := user(t, server, "player")
	statusError(t, other.Call(ctx, "POST", "/v2/node/sync", model.NodeSyncRequest{Report: model.NodeReport{Version: model.Version, Engine: "stopped"}}, nil), 401)
	var stored string
	must(t, s.Pool.QueryRow(ctx, "SELECT key_hash FROM enrollment_keys WHERE node_id=$1", n.ID).Scan(&stored))
	if stored == key {
		t.Fatal("plaintext key saved")
	}
	var secretCount int
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM idempotency WHERE response::text LIKE '%'||$1||'%'", key).Scan(&secretCount))
	if secretCount != 0 {
		t.Fatal("key in idempotency cache")
	}
}
func TestEnrollmentConcurrentConsumptionAndRecovery(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	second := apiServer(t, s, ca)
	n, key := testNode(t, s)
	ctx := context.Background()
	const count = 8
	var wg sync.WaitGroup
	results := make(chan *client.API, count)
	for j := 0; j < count; j++ {
		a := nodeAPI(t, server)
		if j%2 == 1 {
			a = nodeAPI(t, second)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if a.Enroll(ctx, key) == nil {
				results <- a
			}
		}()
	}
	wg.Wait()
	close(results)
	var winner *client.API
	success := 0
	for a := range results {
		winner = a
		success++
	}
	if success != 1 {
		t.Fatalf("consumed %d times", success)
	}
	recovered := client.NewAPI(winner.Identity)
	must(t, recovered.Authenticate(ctx))
	var sync model.NodeSync
	must(t, recovered.Call(ctx, "POST", "/v2/node/sync", model.NodeSyncRequest{Report: model.NodeReport{Version: model.Version, Engine: "stopped"}}, &sync))
	if sync.Node.ID != n.ID {
		t.Fatal("lost registration")
	}
}
func TestDisableReplaceAndAddressQuarantine(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	n, key := testNode(t, s)
	a := nodeAPI(t, server)
	ctx := context.Background()
	must(t, a.Enroll(ctx, key))
	pub := make([]byte, 32)
	pub[0] = 9
	var lease model.Lease
	must(t, a.Call(ctx, "POST", "/v2/node/lease", model.LeaseRequest{PublicKey: pub}, &lease))
	action := func(kind string) {
		must(t, s.Write(ctx, func(tx pgx.Tx) error { _, e := s.nodeAction(ctx, tx, "test", n.ID, kind); return e }))
	}
	action("disable")
	statusError(t, a.Call(ctx, "POST", "/v2/node/lease", model.LeaseRequest{PublicKey: pub}, nil), 403)
	must(t, a.Call(ctx, "POST", "/v2/node/sync", model.NodeSyncRequest{Report: model.NodeReport{Version: model.Version, Engine: "stopped"}}, nil))
	var revoked bool
	must(t, s.Pool.QueryRow(ctx, "SELECT revoked FROM certificates WHERE fingerprint=$1", lease.Fingerprint).Scan(&revoked))
	if !revoked {
		t.Fatal("certificate not revoked")
	}
	action("resume")
	action("replace")
	statusError(t, client.NewAPI(a.Identity).Authenticate(ctx), 403)
	var quarantine bool
	must(t, s.Pool.QueryRow(ctx, "SELECT release_after>now() FROM addresses WHERE ip=$1", lease.IP).Scan(&quarantine))
	if !quarantine {
		t.Fatal("address released too soon")
	}
	newKey, err := s.IssueEnrollmentKey(ctx, n.ID, "test")
	must(t, err)
	replacement := nodeAPI(t, server)
	must(t, replacement.Enroll(ctx, newKey))
	var fresh model.Lease
	must(t, replacement.Call(ctx, "POST", "/v2/node/lease", model.LeaseRequest{PublicKey: pub}, &fresh))
	if fresh.IP == lease.IP || fresh.Node.Generation != 2 {
		t.Fatal("replacement reused live identity/address")
	}
}
func TestRevokedKeyAndConfigChange(t *testing.T) {
	s, ca := database(t)
	srv := apiServer(t, s, ca)
	ctx := context.Background()
	n, key := testNode(t, s)
	fresh, err := s.IssueEnrollmentKey(ctx, n.ID, "test")
	must(t, err)
	statusError(t, nodeAPI(t, srv).Enroll(ctx, key), 403)
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		c := n.Config()
		c.Address = "127.0.0.1:4244"
		_, e := s.updateNode(ctx, tx, "test", n.ID, c, n.Revision)
		return e
	}))
	statusError(t, nodeAPI(t, srv).Enroll(ctx, fresh), 403)
	expired, err := s.IssueEnrollmentKey(ctx, n.ID, "test")
	must(t, err)
	_, err = s.Pool.Exec(ctx, "UPDATE enrollment_keys SET expires_at=now()-interval '1 minute'")
	must(t, err)
	statusError(t, nodeAPI(t, srv).Enroll(ctx, expired), 403)
}
func TestChallengeScopeCannotBeSwapped(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	_, key := testNode(t, s)
	i, err := client.NewIdentity("https://example.com", "node")
	must(t, err)
	c, err := s.challenge(ctx, model.ChallengeRequest{DeviceID: i.ID(), Name: i.Name, PublicKey: ed25519.PrivateKey(i.PrivateKey).Public().(ed25519.PublicKey)}, "enrollment", key)
	must(t, err)
	sig := ed25519.Sign(i.PrivateKey, append([]byte("nodelane-auth-v2:enrollment:"+c.ID+":"), c.Nonce...))
	if _, err = s.verify(ctx, model.VerifyRequest{ID: c.ID, Signature: sig}, "player"); err != ErrUnauthorized {
		t.Fatal("scope confusion")
	}
	_, err = s.verify(ctx, model.VerifyRequest{ID: c.ID, Signature: sig}, "enrollment")
	must(t, err)
}

func TestNodeEnrollmentDrainAndInfrastructureLease(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	ctx := context.Background()
	var n model.Node
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		var e error
		n, e = s.createNode(ctx, tx, "test", model.NodeConfig{Name: "lh", Region: "test", Address: "127.0.0.1:4242", Lighthouse: true, Relay: true})
		return e
	}))
	token, err := s.IssueEnrollmentKey(ctx, n.ID, "test")
	must(t, err)
	i, err := client.NewIdentity(server.URL, "node")
	must(t, err)
	i.Node = true
	node := client.NewAPI(i)
	must(t, node.Enroll(ctx, token))
	otherID, err := client.NewIdentity(server.URL, "other")
	must(t, err)
	otherID.Node = true
	statusError(t, client.NewAPI(otherID).Enroll(ctx, token), 403)
	_, pub, err := pki.TunnelKey()
	must(t, err)
	var l model.Lease
	must(t, node.Call(ctx, "POST", "/v2/node/lease", model.LeaseRequest{PublicKey: pub}, &l))
	if l.Node == nil || l.Node.ID != n.ID || l.Node.DeviceID != i.ID() {
		t.Fatal("incorrect binding")
	}
	var snap model.NodeSync
	must(t, node.Call(ctx, "POST", "/v2/node/sync", model.NodeSyncRequest{Report: model.NodeReport{Version: model.Version, Engine: "running"}}, &snap))
	if len(snap.Snapshot.Nodes) != 1 {
		t.Fatal("missing node")
	}
	must(t, s.Write(ctx, func(tx pgx.Tx) error { _, e := s.nodeAction(ctx, tx, "test", n.ID, "drain"); return e }))
	must(t, node.Call(ctx, "POST", "/v2/node/sync", model.NodeSyncRequest{Report: model.NodeReport{Version: model.Version, Engine: "running"}}, &snap))
	if len(snap.Snapshot.Nodes) != 1 || !snap.Snapshot.Nodes[0].Draining {
		t.Fatal("drain must preserve authorization and remove candidacy")
	}
}

func TestNodeDesiredAndAppliedEndpointAndOperationDedup(t *testing.T) {
	s, ca := database(t)
	ctx := context.Background()
	server := apiServer(t, s, ca)
	n, key := testNode(t, s)
	a := nodeAPI(t, server)
	must(t, a.Enroll(ctx, key))
	c := n.Config()
	report := model.NodeReport{Version: model.Version, Engine: "running", AppliedRevision: 1, AppliedConfig: &c}
	var sync model.NodeSync
	must(t, a.Call(ctx, "POST", "/v2/node/sync", model.NodeSyncRequest{Generation: 1, Report: report}, &sync))
	fresh := c
	fresh.Address = "127.0.0.1:4545"
	must(t, s.Write(ctx, func(tx pgx.Tx) error { _, e := s.updateNode(ctx, tx, "test", n.ID, fresh, 1); return e }))
	must(t, a.Call(ctx, "POST", "/v2/node/sync", model.NodeSyncRequest{Generation: 1, Report: report}, &sync))
	if sync.Node.Address != fresh.Address || sync.Node.Report.AppliedRevision != 1 || len(sync.Snapshot.Nodes) != 1 || sync.Snapshot.Nodes[0].Address != c.Address {
		t.Fatal("unapplied endpoint advertised")
	}
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		_, e := s.updateNode(ctx, tx, "test", n.ID, fresh, 1)
		if e != ErrConflict {
			t.Fatal("stale revision accepted")
		}
		return nil
	}))
	var op model.NodeOperation
	must(t, s.Write(ctx, func(tx pgx.Tx) error { var e error; op, e = s.nodeAction(ctx, tx, "test", n.ID, "drain"); return e }))
	report.AppliedRevision = op.Revision
	report.AppliedConfig = &fresh
	input := model.NodeSyncRequest{Generation: 1, Report: report, Results: []model.OperationResult{{ID: op.ID, State: "succeeded"}}}
	must(t, a.Call(ctx, "POST", "/v2/node/sync", input, &sync))
	var count int
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM admin_events WHERE kind='node.operation_result'").Scan(&count))
	must(t, a.Call(ctx, "POST", "/v2/node/sync", input, &sync))
	var again int
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM admin_events WHERE kind='node.operation_result'").Scan(&again))
	if count != again {
		t.Fatal("duplicate result repeated audit")
	}
	if len(sync.Snapshot.Nodes) != 1 || !sync.Snapshot.Nodes[0].Draining {
		t.Fatal("drain removed authorization or retained candidacy")
	}
	pub := make([]byte, 32)
	pub[0] = 9
	must(t, a.Call(ctx, "POST", "/v2/node/lease", model.LeaseRequest{PublicKey: pub}, nil))
	input.Generation = 2
	statusError(t, a.Call(ctx, "POST", "/v2/node/sync", input, nil), 409)
	input.Generation = 1
	input.Report.AppliedRevision = 999
	statusError(t, a.Call(ctx, "POST", "/v2/node/sync", input, nil), 400)
}
