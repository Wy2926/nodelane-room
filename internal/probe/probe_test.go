package probe

import (
	"context"
	"testing"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

func TestProbeAndDiscoveryMembership(t *testing.T) {
	ads := make(chan string, 2)
	a, err := Start("127.0.0.2", "127.0.0.0/8", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Start("127.0.0.3", "127.0.0.0/8", false, func(_, _, ep string) { ads <- ep })
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
	ep := "0123456789abcdef0123456789abcdef"
	if err = a.Advertise("127.0.0.3", ep); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-ads:
		if got != ep {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("discovery not delivered")
	}
	_ = a.send("127.0.0.3", Packet{Version: 1, Type: "advertisement", Room: "different", Endpoint: ep})
	select {
	case <-ads:
		t.Fatal("cross-room discovery accepted")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestProbeWindowExpiry(t *testing.T) {
	now := time.Now()
	s := &Service{samples: map[string][]sample{"peer": {{at: now.Add(-61 * time.Second), ok: false}, {at: now.Add(-40 * time.Second), rtt: 10 * time.Millisecond, ok: true}, {at: now.Add(-5 * time.Second), ok: false}}}}
	rtt, loss := s.Stats("peer")
	if rtt == nil || *rtt != 10 || loss == nil || *loss != 50 {
		t.Fatalf("window statistics: %v %v", rtt, loss)
	}
	s.samples["peer"] = []sample{{at: now.Add(-61 * time.Second), ok: true}}
	rtt, loss = s.Stats("peer")
	if rtt != nil || loss != nil || !s.LastSample("peer").IsZero() {
		t.Fatal("stale probes remained current")
	}
}
