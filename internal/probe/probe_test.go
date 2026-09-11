package probe

import (
	"context"
	"testing"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

func TestProbeMembership(t *testing.T) {
	a, err := Start("127.0.0.2", "127.0.0.0/8", false)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Start("127.0.0.3", "127.0.0.0/8", false)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	snap := model.Snapshot{Room: &model.Room{ID: "room"}, Members: []model.Member{{IP: "127.0.0.2"}, {IP: "127.0.0.3"}}}
	a.Update(snap)
	b.Update(snap)
	if _, err = a.Ping(context.Background(), "127.0.0.3"); err != nil {
		t.Fatal(err)
	}
	rtt, loss := a.Stats("127.0.0.3")
	if rtt == nil || loss == nil || *loss != 0 {
		t.Fatal("missing measured statistics")
	}
	if _, err = a.Ping(context.Background(), "127.0.0.4"); err == nil {
		t.Fatal("probed nonmember")
	}
	a.Update(model.Snapshot{})
	if _, err = a.Ping(context.Background(), "127.0.0.3"); err == nil {
		t.Fatal("revoked peer still authorized")
	}
	if rtt, loss := a.Stats("127.0.0.3"); rtt != nil || loss != nil {
		t.Fatal("revoked peer measurements retained")
	}
}

func TestProbeWindowExpiry(t *testing.T) {
	now := time.Now()
	s := &Service{samples: map[string][]sample{"peer": {{at: now.Add(-31 * time.Second), ok: false}, {at: now.Add(-20 * time.Second), rtt: 10 * time.Millisecond, ok: true}, {at: now.Add(-5 * time.Second), ok: false}}}}
	rtt, loss := s.Stats("peer")
	if rtt == nil || *rtt != 10 || loss == nil || *loss != 50 {
		t.Fatalf("window statistics: %v %v", rtt, loss)
	}
	s.samples["peer"] = []sample{{at: now.Add(-31 * time.Second), ok: true}}
	rtt, loss = s.Stats("peer")
	if rtt != nil || loss != nil || !s.LastSample("peer").IsZero() {
		t.Fatal("stale probes remained current")
	}
}

func TestProbeAllLossAndNoSamples(t *testing.T) {
	s := &Service{samples: map[string][]sample{"peer": {{at: time.Now(), ok: false}}}}
	rtt, loss := s.Stats("peer")
	if rtt != nil || loss == nil || *loss != 100 {
		t.Fatal("all lost probes must report 100% loss without an RTT")
	}
	if rtt, loss = s.Stats("unknown"); rtt != nil || loss != nil {
		t.Fatal("no probes must not become zero loss")
	}
}

func TestCancelledProbeDoesNotCountAsLoss(t *testing.T) {
	s, err := Start("127.0.0.22", "127.0.0.0/8", false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Update(model.Snapshot{Members: []model.Member{{IP: "127.0.0.23"}}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err = s.Ping(ctx, "127.0.0.23"); err == nil {
		t.Fatal("expected cancelled probe")
	}
	if rtt, loss := s.Stats("127.0.0.23"); rtt != nil || loss != nil {
		t.Fatal("cancellation was recorded as packet loss")
	}
}
