package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"runtime"
	"testing"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/engine"
	"github.com/nodelane/nodelane-room/internal/localapi"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
	"github.com/slackhq/nebula/cert"
	"github.com/slackhq/nebula/overlay"
)

func TestCredentialExpiryWhileControlRequestIsBlocked(t *testing.T) {
	testExpiryWhileBlocked(t, false)
}

func TestMandatoryUpdateDeadlineWhileControlRequestIsBlocked(t *testing.T) {
	testExpiryWhileBlocked(t, true)
}

func testExpiryWhileBlocked(t *testing.T, mandatory bool) {
	t.Helper()
	requested := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case requested <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	r, err := New(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	r.engine = engine.NewWithDeviceFactory(slog.New(slog.NewTextHandler(io.Discard, nil)), overlay.NewUserDeviceFromConfig)
	i, err := device.NewIdentity(server.URL, "alice")
	if err != nil {
		t.Fatal(err)
	}
	i.RoomID = "test-room"
	r.identity = i
	r.api = client.NewAPI(i)
	ca, err := pki.Generate(netip.MustParsePrefix(model.DefaultPool))
	if err != nil {
		t.Fatal(err)
	}
	key, pub, err := pki.TunnelKey()
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(3 * time.Second).Truncate(time.Second)
	if mandatory {
		deadline := time.Now().Add(time.Second)
		r.updateState.Policy = &model.UpdatePolicy{MinimumVersion: "9.0.0", EffectiveAt: &deadline}
		until = time.Now().Add(time.Minute).Truncate(time.Second)
	}
	leaf, err := (&cert.TBSCertificate{Version: cert.Version2, Name: i.ID(), Networks: []netip.Prefix{netip.MustParsePrefix("10.203.0.2/16")}, Groups: []string{"room:test-room"}, PublicKey: pub, Curve: cert.Curve_CURVE25519, NotBefore: time.Now().Add(-time.Minute), NotAfter: until}).Sign(ca.Certificate, cert.Curve_CURVE25519, ca.Key)
	if err != nil {
		t.Fatal(err)
	}
	pem, err := leaf.MarshalPEM()
	if err != nil {
		t.Fatal(err)
	}
	r.lease = model.Lease{IP: "10.203.0.2", Network: model.DefaultPool, RoomID: i.RoomID, CA: ca.PEM, Certificate: string(pem), ExpiresAt: until}
	s := model.Snapshot{Game: &model.Game{Network: model.GameNetwork{Version: 1}}, Room: &model.Room{ID: i.RoomID}, Members: []model.Member{{DeviceID: i.ID(), IP: r.lease.IP}}}
	if err = r.engine.Apply(engine.Config{Lease: r.lease, Snapshot: s, PrivateKey: key, DeviceID: i.ID(), DisableTUN: true}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	select {
	case <-requested:
	case <-time.After(time.Second):
		t.Fatal("no blocked control request")
	}
	deadline := time.Now().Add(4 * time.Second)
	for r.engine.Running() && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if r.engine.Running() {
		t.Fatal("data plane survived credential expiration")
	}
	cancel()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runtime did not stop")
	}
}

func TestSignedOutIdentityStaysSignedOutAfterRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires the installed protected ProgramData state directory; exercised in isolated Linux")
	}
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r, err := New(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	i, err := device.NewIdentity("https://example.invalid", "player")
	if err != nil {
		t.Fatal(err)
	}
	i.SignedOut = true
	if err = r.persist(i); err != nil {
		t.Fatal(err)
	}
	restored, err := New(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	if restored.api != nil || restored.status.Control != "signed_out" || restored.identity.ID() != i.ID() {
		t.Fatal("signed out identity was silently restored")
	}
	if err = restored.step(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLogoutOfAlreadyRevokedDevicePersistsSignedOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires protected ProgramData state; exercised in isolated Linux")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/v2/auth/challenge" {
			_ = json.NewEncoder(w).Encode(model.NewResult("ok", "control", client.ID(), model.Challenge{ID: client.ID(), Nonce: make([]byte, 32)}))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(model.NewResult("auth_device_revoked", "control", client.ID(), nil))
	}))
	defer server.Close()
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r, err := New(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	i, err := device.NewIdentity(server.URL, "player")
	if err != nil {
		t.Fatal(err)
	}
	if err = r.persist(i); err != nil {
		t.Fatal(err)
	}
	r.api = client.NewAPI(i)
	watchStopped := false
	r.watchCancel = func() { watchStopped = true }
	if _, err = r.accountAction(context.Background(), localapi.Request{Action: "account-logout"}); err != nil {
		t.Fatal(err)
	}
	if !watchStopped || r.watchCancel != nil {
		t.Fatal("logout retained the old room watcher")
	}
	restored, err := New(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	if restored.api != nil || !restored.identity.SignedOut {
		t.Fatal("revoked logout left device signed in")
	}
}
