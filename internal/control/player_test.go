package control

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
)

func TestRoomLifecycleAcrossReplicas(t *testing.T) {
	s, ca := database(t)
	one := apiServer(t, s, ca)
	two := apiServer(t, s, ca)
	ctx := context.Background()
	ownerAPI := user(t, one, "owner")
	guest := user(t, two, "guest")
	r := create(t, ownerAPI)
	join(t, guest, r)
	l := lease(t, guest, r.Room.ID)
	if time.Until(l.ExpiresAt) > model.LeaseDuration || l.RoomID != r.Room.ID {
		t.Fatal("lease is not room scoped and bounded")
	}
	var snap model.Snapshot
	must(t, ownerAPI.Call(ctx, "GET", "/v2/rooms/"+r.Room.ID, nil, &snap))
	if len(snap.Members) != 2 {
		t.Fatalf("members: %+v", snap.Members)
	}
	must(t, ownerAPI.Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/transfer", model.MemberRequest{ExpectedRevision: roomRevision(t, s, r.Room.ID), DeviceID: guest.Identity.ID()}, nil))
	statusError(t, ownerAPI.Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/close", model.MemberRequest{ExpectedRevision: roomRevision(t, s, r.Room.ID)}, nil), 403)
	must(t, guest.Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/close", model.MemberRequest{ExpectedRevision: roomRevision(t, s, r.Room.ID)}, nil))
	must(t, ownerAPI.Call(ctx, "GET", "/v2/rooms/"+r.Room.ID, nil, &snap))
	if snap.Room != nil || snap.Self.Reason != "room_closed" || len(snap.Members) != 0 || len(snap.Blocklist) != 0 {
		t.Fatalf("not closed: %+v", snap)
	}
	var found bool
	must(t, s.Pool.QueryRow(ctx, "SELECT revoked FROM certificates WHERE fingerprint=$1", l.Fingerprint).Scan(&found))
	if !found {
		t.Fatal("closing did not revoke outstanding certificate")
	}
	var n int
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM addresses WHERE release_after>now()").Scan(&n))
	if n != 2 {
		t.Fatal("addresses must remain quarantined until old certificates expire")
	}
	// Same durable device can build another room after the previous one closes.
	_ = create(t, guest)
}

func TestCapacityAndOneRoomUnderConcurrency(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	host := user(t, server, "host")
	r := create(t, host)
	clients := make([]*client.API, 40)
	for i := range clients {
		clients[i] = user(t, server, "guest")
	}
	results := make(chan error, len(clients))
	var wg sync.WaitGroup
	for _, a := range clients {
		wg.Add(1)
		go func(a *client.API) {
			defer wg.Done()
			results <- a.Call(context.Background(), "POST", "/v2/rooms/join", model.JoinRequest{Code: r.Invitation.Code}, nil)
		}(a)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			statusError(t, err, 409)
		}
	}
	if success != model.RoomCapacity-1 {
		t.Fatalf("joined %d, want %d", success, model.RoomCapacity-1)
	}
	var n int
	must(t, s.Pool.QueryRow(context.Background(), "SELECT count(DISTINCT ip) FROM members WHERE active").Scan(&n))
	if n != model.RoomCapacity {
		t.Fatal("IP allocation collided")
	}
	statusError(t, host.Call(context.Background(), "POST", "/v2/rooms", model.RoomRequest{ExpectedGameRevision: 1, Name: "second", Game: "custom"}, nil), 409)
}
func TestKickRevokesAllRotatedCertificatesAndInvitation(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	host := user(t, server, "host")
	guest := user(t, server, "guest")
	r := create(t, host)
	join(t, guest, r)
	_ = lease(t, guest, r.Room.ID)
	_ = lease(t, guest, r.Room.ID)
	ctx := context.Background()
	must(t, host.Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/kick", model.MemberRequest{ExpectedRevision: roomRevision(t, s, r.Room.ID), DeviceID: guest.Identity.ID()}, nil))
	statusError(t, guest.Call(ctx, "POST", "/v2/rooms/join", model.JoinRequest{Code: r.Invitation.Code}, nil), 403)
	_, pub, err := pki.TunnelKey()
	must(t, err)
	statusError(t, guest.Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/lease", model.LeaseRequest{PublicKey: pub}, nil), 403)
	var n int
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM certificates WHERE device_id=$1 AND revoked", guest.Identity.ID()).Scan(&n))
	if n != 2 {
		t.Fatalf("revoked %d certificates", n)
	}
	var inv model.Invitation
	must(t, host.Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/invite", model.MemberRequest{ExpectedRevision: roomRevision(t, s, r.Room.ID)}, &inv))
	other := user(t, server, "other")
	statusError(t, other.Call(ctx, "POST", "/v2/rooms/join", model.JoinRequest{Code: r.Invitation.Code}, nil), 403)
}
func TestAuthenticationReplayAndIdentityBinding(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	i, err := device.NewIdentity("http://127.0.0.1", "alice")
	must(t, err)
	pub := ed25519.PrivateKey(i.PrivateKey).Public().(ed25519.PublicKey)
	in := model.ChallengeRequest{DeviceID: i.ID(), Name: i.Name, PublicKey: pub}
	c, err := s.challenge(ctx, in, "guest", "")
	must(t, err)
	sig := ed25519.Sign(i.PrivateKey, append([]byte("nodelane-auth-v2:guest:"+c.ID+":"), c.Nonce...))
	session, err := s.verify(ctx, model.VerifyRequest{ID: c.ID, Signature: sig}, "guest")
	must(t, err)
	if session.DeviceID != i.ID() {
		t.Fatal("identity mismatch")
	}
	_, err = s.Verify(ctx, model.VerifyRequest{ID: c.ID, Signature: sig})
	if !model.IsCode(err, "auth_challenge_unusable") {
		t.Fatal("replayed challenge accepted")
	}
	in.DeviceID = "someone-else"
	_, err = s.Challenge(ctx, in)
	if !errors.Is(err, ErrInvalid) {
		t.Fatal("public key not bound to ID")
	}
	c, err = s.Challenge(ctx, model.ChallengeRequest{DeviceID: i.ID(), Name: i.Name, PublicKey: pub})
	must(t, err)
	_, err = s.Verify(ctx, model.VerifyRequest{ID: c.ID, Signature: make([]byte, 64)})
	if !model.IsCode(err, "auth_proof_invalid") {
		t.Fatal("forged signature accepted")
	}
}
func TestExpiryReclaimAndRoomAuthorization(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	host := user(t, server, "host")
	stranger := user(t, server, "stranger")
	r := create(t, host)
	ctx := context.Background()
	path := "/v2/rooms/" + r.Room.ID
	statusError(t, stranger.Call(ctx, "GET", path, nil, nil), 404)
	l := lease(t, host, r.Room.ID)
	_, err := s.Pool.Exec(ctx, "UPDATE rooms SET expires_at=now()-interval '1 second' WHERE id=$1", r.Room.ID)
	must(t, err)
	must(t, s.Sweep(ctx))
	var revoked bool
	must(t, s.Pool.QueryRow(ctx, "SELECT revoked FROM certificates WHERE fingerprint=$1", l.Fingerprint).Scan(&revoked))
	if !revoked {
		t.Fatal("room expiry did not revoke lease")
	}
	var count int
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM members WHERE active").Scan(&count))
	if count != 0 {
		t.Fatal("expired members retained")
	}
	_, err = s.Pool.Exec(ctx, "UPDATE addresses SET release_after=now()-interval '1 second'")
	must(t, err)
	must(t, s.Sweep(ctx))
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM addresses").Scan(&count))
	if count != 0 {
		t.Fatal("quarantined addresses never reclaimed")
	}
}

func TestRoomHeartbeatAutoRenewalKeepsAuthorizationBounded(t *testing.T) {
	s, ca := database(t)
	a := user(t, apiServer(t, s, ca), "host")
	r := create(t, a)
	ctx := context.Background()
	path := "/v2/rooms/" + r.Room.ID
	var before time.Time
	must(t, s.Pool.QueryRow(ctx, "UPDATE rooms SET expires_at=now()+interval '2 minutes' WHERE id=$1 RETURNING expires_at", r.Room.ID).Scan(&before))
	l := lease(t, a, r.Room.ID)
	if !l.ExpiresAt.Before(before.Add(time.Second)) {
		t.Fatal("initial lease must remain bounded by room expiry")
	}
	revision := roomRevision(t, s, r.Room.ID)
	var result struct {
		AcceptedAt time.Time `json:"accepted_at"`
		ValidUntil time.Time `json:"membership_valid_until"`
	}
	must(t, a.Call(ctx, "POST", path+"/heartbeat", model.HeartbeatRequest{LANVersion: model.LANVersion}, &result))
	if !result.ValidUntil.Equal(result.AcceptedAt.Add(45 * time.Second)) {
		t.Fatal("renewal must retain the 45 second heartbeat authorization limit")
	}
	var snap model.Snapshot
	must(t, a.Call(ctx, "GET", path, nil, &snap))
	if snap.Room == nil || !snap.Room.ExpiresAt.Equal(before.Add(24*time.Hour)) || snap.Room.Revision != revision+1 {
		t.Fatal("heartbeat did not publish exactly one day of renewal with a new revision")
	}
	var certificateExpiry time.Time
	must(t, s.Pool.QueryRow(ctx, "SELECT expires_at FROM certificates WHERE fingerprint=$1", l.Fingerprint).Scan(&certificateExpiry))
	if !certificateExpiry.Equal(l.ExpiresAt) {
		t.Fatal("room renewal extended an already issued certificate")
	}
	must(t, a.Call(ctx, "POST", path+"/heartbeat", model.HeartbeatRequest{LANVersion: model.LANVersion}, nil))
	must(t, s.Sweep(ctx))
	var after time.Time
	must(t, s.Pool.QueryRow(ctx, "SELECT expires_at FROM rooms WHERE id=$1", r.Room.ID).Scan(&after))
	if !after.Equal(before.Add(24*time.Hour)) || roomRevision(t, s, r.Room.ID) != revision+1 {
		t.Fatal("normal heartbeats or maintenance renewed the room again")
	}
}

func TestRoomAutoRenewalAcrossConcurrentHeartbeatsAndSweeps(t *testing.T) {
	s, ca := database(t)
	one, two := apiServer(t, s, ca), apiServer(t, s, ca)
	clients := []*client.API{user(t, one, "host"), user(t, two, "guest"), user(t, one, "other")}
	r := create(t, clients[0])
	join(t, clients[1], r)
	join(t, clients[2], r)
	ctx := context.Background()
	var before time.Time
	must(t, s.Pool.QueryRow(ctx, "UPDATE rooms SET expires_at=now()+interval '1 hour' WHERE id=$1 RETURNING expires_at", r.Room.ID).Scan(&before))
	revision := roomRevision(t, s, r.Room.ID)
	results := make(chan error, 16)
	var wg sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%4 == 0 {
				results <- s.Sweep(ctx)
				return
			}
			results <- clients[i%len(clients)].Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/heartbeat", model.HeartbeatRequest{LANVersion: model.LANVersion}, nil)
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		must(t, err)
	}
	var after time.Time
	must(t, s.Pool.QueryRow(ctx, "SELECT expires_at FROM rooms WHERE id=$1", r.Room.ID).Scan(&after))
	if !after.Equal(before.Add(24*time.Hour)) || roomRevision(t, s, r.Room.ID) != revision+1 {
		t.Fatal("concurrent replicas must extend the room exactly once")
	}
	var changes, audits int
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE room_id=$1 AND kind='room_renewed'", r.Room.ID).Scan(&changes))
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM admin_events WHERE target=$1 AND kind='room.renewed'", r.Room.ID).Scan(&audits))
	if changes != 1 || audits != 1 {
		t.Fatalf("expected one durable change and audit, got %d and %d", changes, audits)
	}
}

func TestRoomSweepRenewsOnlyAuthorizedOnlineRooms(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		adjust string
		renew  bool
	}{
		{name: "online", renew: true},
		{name: "offline", adjust: "UPDATE members SET last_seen=now()-interval '46 seconds' WHERE room_id=$1"},
		{name: "left", adjust: "UPDATE members SET active=false WHERE room_id=$1"},
		{name: "revoked device", adjust: "UPDATE user_devices SET revoked=true WHERE device_id IN (SELECT device_id FROM members WHERE room_id=$1)"},
		{name: "expired device", adjust: "UPDATE user_devices SET expires_at=now()-interval '1 second' WHERE device_id IN (SELECT device_id FROM members WHERE room_id=$1)"},
		{name: "deleted user", adjust: "UPDATE users SET state='deleted' WHERE id=(SELECT owner_user_id FROM rooms WHERE id=$1)"},
		{name: "creation restricted user", adjust: "UPDATE users SET state='disabled' WHERE id=(SELECT owner_user_id FROM rooms WHERE id=$1)", renew: true},
		{name: "closed", adjust: "UPDATE rooms SET closed=true WHERE id=$1"},
		{name: "expired", adjust: "UPDATE rooms SET expires_at=now()-interval '1 second' WHERE id=$1"},
		{name: "outside final hour", adjust: "UPDATE rooms SET expires_at=now()+interval '61 minutes' WHERE id=$1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := user(t, server, "host")
			r := create(t, a)
			_, err := s.Pool.Exec(ctx, "UPDATE rooms SET expires_at=now()+interval '30 minutes' WHERE id=$1", r.Room.ID)
			must(t, err)
			if tc.adjust != "" {
				_, err = s.Pool.Exec(ctx, tc.adjust, r.Room.ID)
				must(t, err)
			}
			var before, after time.Time
			must(t, s.Pool.QueryRow(ctx, "SELECT expires_at FROM rooms WHERE id=$1", r.Room.ID).Scan(&before))
			must(t, s.Sweep(ctx))
			must(t, s.Pool.QueryRow(ctx, "SELECT expires_at FROM rooms WHERE id=$1", r.Room.ID).Scan(&after))
			want := before
			if tc.renew {
				want = want.Add(24 * time.Hour)
			}
			if !after.Equal(want) {
				t.Fatalf("expiry changed by %s, want %s", after.Sub(before), want.Sub(before))
			}
		})
	}
}

func TestIdempotencyAndSSESnapshotRecovery(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	a := user(t, server, "owner")
	ctx := context.Background()
	var challenge model.Challenge
	pub := ed25519.PrivateKey(a.Identity.PrivateKey).Public().(ed25519.PublicKey)
	challenge, err := s.Challenge(ctx, model.ChallengeRequest{DeviceID: a.Identity.ID(), Name: "owner", PublicKey: pub})
	must(t, err)
	session, err := s.Verify(ctx, model.VerifyRequest{ID: challenge.ID, Signature: ed25519.Sign(a.Identity.PrivateKey, append([]byte("nodelane-auth-v2:player:"+challenge.ID+":"), challenge.Nonce...))})
	must(t, err)
	body := []byte(`{"name":"once","game":"custom","expected_game_revision":1}`)
	key := randomID()
	deadline := time.Now().UTC().Add(50 * time.Minute)
	invoke := func(path string, body []byte) (int, []byte) {
		req, e := http.NewRequest("POST", server.URL+path, bytes.NewReader(body))
		must(t, e)
		req.Header.Set("Authorization", "Bearer "+session.Token)
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set(model.ContractHeader, model.Contract)
		req.Header.Set(model.DeadlineHeader, deadline.Format(time.RFC3339Nano))
		resp, e := http.DefaultClient.Do(req)
		must(t, e)
		defer resp.Body.Close()
		b, e := io.ReadAll(resp.Body)
		must(t, e)
		return resp.StatusCode, responseData(t, b)
	}
	status, first := invoke("/v2/rooms", body)
	if status != 201 {
		t.Fatalf("%d: %s", status, first)
	}
	status, second := invoke("/v2/rooms", body)
	if status != 201 {
		t.Fatal(string(second))
	}
	var firstObject, secondObject any
	must(t, json.Unmarshal(first, &firstObject))
	must(t, json.Unmarshal(second, &secondObject))
	f, _ := json.Marshal(firstObject)
	g, _ := json.Marshal(secondObject)
	if !bytes.Equal(f, g) {
		t.Fatal("retry created different result")
	}
	status, _ = invoke("/v2/rooms", []byte(`{"name":"different","game":"custom"}`))
	if status != 409 {
		t.Fatal("idempotency key reused with another operation")
	}
	var r model.RoomResult
	must(t, json.Unmarshal(first, &r))
	status, _ = invoke("/v2/rooms/"+r.Room.ID+"/heartbeat", body)
	if status != 409 {
		t.Fatalf("cross-endpoint replay: %d", status)
	}
	must(t, a.Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/heartbeat", model.HeartbeatRequest{LANVersion: 1}, nil))
	watchCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	seen := false
	_ = a.Watch(watchCtx, r.Room.ID, 99999, func(s model.Snapshot) { seen = s.Room != nil && s.Room.ID == r.Room.ID; cancel() })
	if !seen {
		t.Fatal("SSE failed to recover with a full snapshot")
	}
}
