package control

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

func TestTelemetryWindowAndIsolation(t *testing.T) {
	now := time.Now().UTC()
	var memory telemetryMemory
	id := model.TelemetrySeries{DeviceID: "a", RoomID: "room"}
	for i := 0; i <= 12; i++ {
		at := now.Add(time.Duration(i-12) * 5 * time.Second)
		must(t, memory.put(id, model.NetworkSample{At: at, Peers: []model.LinkSample{{Remote: "1.1.1.1:4242"}}}, at))
	}
	snapshot := memory.snapshot(now, nil)
	if len(snapshot.Series) != 1 || len(snapshot.Series[0].Samples) != 13 {
		t.Fatal("did not retain the full 60-second window")
	}
	snapshot.Series[0].Samples[0].Peers[0].Remote = "changed"
	if memory.snapshot(now, nil).Series[0].Samples[0].Peers[0].Remote == "changed" {
		t.Fatal("reader mutated memory")
	}
	if err := memory.put(id, model.NetworkSample{At: now.Add(time.Second)}, now); err != ErrRateLimited {
		t.Fatalf("rate limit: %v", err)
	}
	must(t, memory.put(id, model.NetworkSample{At: now.Add(-time.Second)}, now))
	if len(memory.snapshot(now, nil).Series[0].Samples) != 13 {
		t.Fatal("replay changed history")
	}
	id.RoomID = "new room"
	must(t, memory.put(id, model.NetworkSample{At: now.Add(5 * time.Second)}, now.Add(5*time.Second)))
	if x := memory.snapshot(now.Add(5*time.Second), nil); len(x.Series[0].Samples) != 1 || x.Series[0].RoomID != id.RoomID {
		t.Fatal("room history leaked across membership")
	}
	if len(memory.snapshot(now.Add(66*time.Second), nil).Series) != 0 || memory.bytes != 0 {
		t.Fatal("expired samples not reclaimed")
	}
}

func TestTelemetryConcurrentReaders(t *testing.T) {
	var memory telemetryMemory
	var wg sync.WaitGroup
	now := time.Now()
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 16; j++ {
				at := now.Add(time.Duration(j) * 5 * time.Second)
				_ = memory.put(model.TelemetrySeries{DeviceID: strings.Repeat(string(rune('a'+i)), 64)}, model.NetworkSample{At: at}, at)
				_ = memory.snapshot(at, nil)
			}
		}(i)
	}
	wg.Wait()
}

func TestTelemetryRejectsInvalidObservations(t *testing.T) {
	now := time.Now()
	valid := func() model.NetworkSample {
		return model.NetworkSample{At: now, Epoch: strings.Repeat("e", 32), Connections: 1, Peers: []model.LinkSample{{DeviceID: strings.Repeat("a", 64), IP: "10.203.0.1", Mode: "direct", Remote: "1.1.1.1:4242"}}}
	}
	if !validSample(valid(), now) {
		t.Fatal("valid sample rejected")
	}
	for name, change := range map[string]func(*model.NetworkSample){
		"old":           func(s *model.NetworkSample) { s.At = now.Add(-time.Minute) },
		"future":        func(s *model.NetworkSample) { s.At = now.Add(time.Minute) },
		"fake country":  func(s *model.NetworkSample) { s.Peers[0].Country = "US" },
		"endpoint":      func(s *model.NetworkSample) { s.Peers[0].Remote = "https://example.com" },
		"loss":          func(s *model.NetworkSample) { v := 101.0; s.Peers[0].LossPercent = &v; s.Peers[0].ProbeAt = now },
		"missing probe": func(s *model.NetworkSample) { v := 0.0; s.Peers[0].RTTMillis = &v },
		"duplicate":     func(s *model.NetworkSample) { s.Connections = 2; s.Peers = append(s.Peers, s.Peers[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			s := valid()
			change(&s)
			if validSample(s, now) {
				t.Fatal("invalid sample accepted")
			}
		})
	}
}

func TestTelemetryAuthorizationAndNoPersistence(t *testing.T) {
	store, admin := newAdmin(t)
	alice := user(t, admin.server, "Alice")
	bob := user(t, admin.server, "Bob")
	other := user(t, admin.server, "Other")
	room := create(t, alice)
	join(t, bob, room)
	foreign := create(t, other)
	snapshot, err := store.adminRoomSnapshot(context.Background(), room.Room.ID)
	must(t, err)
	var bobIP string
	for _, m := range snapshot.Members {
		if m.DeviceID == bob.Identity.ID() {
			bobIP = m.IP
		}
	}
	sample := model.NetworkSample{At: time.Now().UTC(), Epoch: strings.Repeat("e", 32), Connections: 1, Peers: []model.LinkSample{{DeviceID: bob.Identity.ID(), IP: bobIP, Mode: "direct", Remote: "1.1.1.1:40000"}}}
	var before, after int
	must(t, store.Pool.QueryRow(context.Background(), "SELECT count(*) FROM idempotency").Scan(&before))
	must(t, alice.Call(context.Background(), "POST", "/v2/rooms/"+room.Room.ID+"/telemetry", sample, nil))
	must(t, store.Pool.QueryRow(context.Background(), "SELECT count(*) FROM idempotency").Scan(&after))
	if before != after {
		t.Fatal("telemetry persisted in idempotency")
	}
	if err := alice.Call(context.Background(), "POST", "/v2/rooms/"+foreign.Room.ID+"/telemetry", sample, nil); err == nil {
		t.Fatal("cross-room report accepted")
	}
	status, b := admin.request("GET", "/telemetry", nil, false, false)
	if status != 200 {
		t.Fatalf("admin telemetry: %d", status)
	}
	var out model.TelemetrySnapshot
	must(t, json.Unmarshal(b, &out))
	if len(out.Series) != 1 || out.Series[0].RoomID != room.Room.ID || len(out.Series[0].Samples[0].Peers) != 1 {
		t.Fatal("authenticated report missing")
	}
	must(t, bob.Call(context.Background(), "POST", "/v2/rooms/"+room.Room.ID+"/leave", struct{}{}, nil))
	if err := bob.Call(context.Background(), "POST", "/v2/rooms/"+room.Room.ID+"/telemetry", sample, nil); err == nil {
		t.Fatal("departed member report accepted")
	}
}
