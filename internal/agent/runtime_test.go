package agent

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/engine"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
	"github.com/slackhq/nebula/cert"
	"github.com/slackhq/nebula/overlay"
)

func TestCredentialExpiryWhileControlRequestIsBlocked(t *testing.T) {
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
