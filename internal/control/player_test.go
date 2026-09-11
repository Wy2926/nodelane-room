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
	must(t, ownerAPI.Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/transfer", model.MemberRequest{DeviceID: guest.Identity.ID()}, nil))
	statusError(t, ownerAPI.Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/close", model.MemberRequest{}, nil), 403)
	must(t, guest.Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/close", model.MemberRequest{}, nil))
	must(t, ownerAPI.Call(ctx, "GET", "/v2/rooms/"+r.Room.ID, nil, &snap))
	if !snap.Room.Closed || len(snap.Members) != 0 {
		t.Fatalf("not closed: %+v", snap)
	}
	found := false
	for _, f := range snap.Blocklist {
		found = found || f == l.Fingerprint
	}
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
	statusError(t, host.Call(context.Background(), "POST", "/v2/rooms", model.RoomRequest{Name: "second", Game: "custom"}, nil), 409)
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
	must(t, host.Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/kick", model.MemberRequest{DeviceID: guest.Identity.ID()}, nil))
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
	must(t, host.Call(ctx, "POST", "/v2/rooms/"+r.Room.ID+"/invite", model.MemberRequest{}, &inv))
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
	if !errors.Is(err, ErrUnauthorized) {
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
	if !errors.Is(err, ErrUnauthorized) {
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
	statusError(t, stranger.Call(ctx, "GET", path, nil, nil), 403)
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
	body := []byte(`{"name":"once","game":"custom"}`)
	key := randomID()
	invoke := func(path string, body []byte) (int, []byte) {
		req, e := http.NewRequest("POST", server.URL+path, bytes.NewReader(body))
		must(t, e)
		req.Header.Set("Authorization", "Bearer "+session.Token)
		req.Header.Set("Idempotency-Key", key)
		resp, e := http.DefaultClient.Do(req)
		must(t, e)
		defer resp.Body.Close()
		b, e := io.ReadAll(resp.Body)
		must(t, e)
		return resp.StatusCode, b
	}
	status, first := invoke("/v2/rooms", body)
	if status != 200 {
		t.Fatalf("%d: %s", status, first)
	}
	status, second := invoke("/v2/rooms", body)
	if status != 200 {
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
