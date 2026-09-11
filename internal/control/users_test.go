package control

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
)

// This issuer executes the real discovery, browser redirect, PKCE code exchange
// and JWKS signature verification path without external accounts or credentials.
func oidcFixture(t *testing.T, bad string) (*Store, *httptest.Server) {
	t.Helper()
	s, ca := database(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	must(t, err)
	var issuer string
	var mu sync.Mutex
	codes := map[string]url.Values{}
	op := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeJSON(w, 200, map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			writeJSON(w, 200, map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "test", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
		case "/authorize":
			q := r.URL.Query()
			if q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" {
				http.Error(w, "PKCE required", 400)
				return
			}
			code := randomID()
			mu.Lock()
			codes[code] = q
			mu.Unlock()
			http.Redirect(w, r, q.Get("redirect_uri")+"?code="+code+"&state="+url.QueryEscape(q.Get("state")), 302)
		case "/token":
			_ = r.ParseForm()
			mu.Lock()
			q, ok := codes[r.Form.Get("code")]
			delete(codes, r.Form.Get("code"))
			mu.Unlock()
			digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if !ok || base64.RawURLEncoding.EncodeToString(digest[:]) != q.Get("code_challenge") || r.Form.Get("redirect_uri") != q.Get("redirect_uri") {
				http.Error(w, "invalid code", 400)
				return
			}
			claims := map[string]any{"iss": issuer, "sub": "subject-1", "aud": "nodelane-test", "exp": time.Now().Add(time.Minute).Unix(), "iat": time.Now().Unix(), "nonce": q.Get("nonce")}
			if bad == "nonce" {
				claims["nonce"] = "wrong"
			}
			if bad == "aud" {
				claims["aud"] = "other-client"
			}
			if bad == "issuer" {
				claims["iss"] = "https://untrusted.invalid"
			}
			if bad == "azp" {
				claims["azp"] = "another-client"
			}
			if bad == "multi-aud" {
				claims["aud"] = []string{"nodelane-test", "another-client"}
			}
			if bad == "expired" {
				claims["exp"] = time.Now().Add(-time.Hour).Unix()
			}
			header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test"})
			body, _ := json.Marshal(claims)
			unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
			sum := sha256.Sum256([]byte(unsigned))
			signature, e := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
			if e != nil {
				http.Error(w, "sign failed", 500)
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "test-only", "token_type": "Bearer", "id_token": unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)})
		default:
			http.NotFound(w, r)
		}
	}))
	issuer = op.URL
	t.Cleanup(op.Close)
	_, err = s.Pool.Exec(context.Background(), `INSERT INTO oidc_settings(id,issuer,client_id,client_secret,enabled) VALUES(1,$1,'nodelane-test','test-secret',true)`, issuer)
	must(t, err)
	control := &Server{Store: s, CA: ca, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	server := httptest.NewServer(control.Handler())
	control.PublicURL = server.URL
	t.Cleanup(server.Close)
	return s, server
}

func confirmLogin(t *testing.T, server *httptest.Server, attempt model.LoginAttempt) int {
	t.Helper()
	jar, err := cookiejar.New(nil)
	must(t, err)
	browser := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	resp, err := browser.Get(attempt.URL)
	must(t, err)
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	must(t, err)
	if resp.StatusCode != 200 {
		return resp.StatusCode
	}
	// A real browser derives the form's Origin from this policy; Go does not.
	if resp.Header.Get("Referrer-Policy") != "strict-origin" {
		t.Fatal("confirmation must preserve Origin without referring callback code/state")
	}
	match := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindSubmatch(b)
	if len(match) != 2 {
		t.Fatalf("confirmation form missing (status %d)", resp.StatusCode)
	}
	req, err := http.NewRequest("POST", server.URL+"/v2/auth/oidc/confirm", strings.NewReader(url.Values{"id": {attempt.ID}, "csrf": {string(match[1])}}.Encode()))
	must(t, err)
	req.Header.Set("Origin", server.URL)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = browser.Do(req)
	must(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestOIDCConfirmationRequiresOriginCookieAndCSRF(t *testing.T) {
	_, server := oidcFixture(t, "")
	a := user(t, server, "guest")
	proof := randomID() + randomID()
	attempt, err := a.BeginLogin(context.Background(), proof, true)
	must(t, err)
	jar, err := cookiejar.New(nil)
	must(t, err)
	browser := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	resp, err := browser.Get(attempt.URL)
	must(t, err)
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	must(t, err)
	match := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindSubmatch(b)
	if resp.StatusCode != 200 || len(match) != 2 {
		t.Fatalf("confirmation form missing (status %d)", resp.StatusCode)
	}
	var cookie string
	for _, c := range jar.Cookies(resp.Request.URL) {
		if c.Name == loginCookie(attempt.ID) {
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatal("browser binding cookie missing")
	}
	csrf := string(match[1])
	wrongCookie := randomID() + randomID()
	for _, tc := range []struct {
		name, origin, site, cookie, csrf string
		status                           int
	}{
		{"missing origin", "", "same-origin", cookie, csrf, 403},
		{"null origin", "null", "same-origin", cookie, csrf, 403},
		{"foreign origin", "https://untrusted.invalid", "same-site", cookie, csrf, 403},
		{"cross-site fetch", server.URL, "cross-site", cookie, csrf, 403},
		{"missing csrf", server.URL, "same-origin", cookie, "", 403},
		{"wrong csrf", server.URL, "same-origin", cookie, hash("wrong"), 403},
		{"missing cookie", server.URL, "same-origin", "", csrf, 403},
		{"other browser", server.URL, "same-origin", wrongCookie, hash("confirm:" + wrongCookie), 403},
		{"valid confirmation", server.URL, "same-origin", cookie, csrf, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", server.URL+"/v2/auth/oidc/confirm", strings.NewReader(url.Values{"id": {attempt.ID}, "csrf": {tc.csrf}}.Encode()))
			must(t, err)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Sec-Fetch-Site", tc.site)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: loginCookie(attempt.ID), Value: tc.cookie})
			}
			resp, err := server.Client().Do(req)
			must(t, err)
			resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Fatalf("confirmation: got %d, want %d", resp.StatusCode, tc.status)
			}
			result, err := a.ClaimLogin(context.Background(), attempt.ID, proof)
			must(t, err)
			if tc.status == 403 {
				if result.State != "verified" || result.Session != nil {
					t.Fatal("rejected confirmation changed device authorization")
				}
			} else if result.State != "ready" || result.Session == nil || result.Session.User.Kind != "registered" {
				t.Fatal("valid confirmation did not authorize device")
			}
		})
	}
}

func TestGuestUpgradePreservesUserRoomAndRejectsMerge(t *testing.T) {
	s, server := oidcFixture(t, "")
	a := user(t, server, "same nickname")
	b := user(t, server, "same nickname")
	room := create(t, a)
	if a.Account().ID == b.Account().ID || a.Account().ID == a.Identity.ID() {
		t.Fatal("nickname or device used as account identity")
	}
	if room.Room.Capacity != 4 || room.Room.OwnerUserID != a.Account().ID {
		t.Fatal("wrong default capacity or owner")
	}
	original := a.Account().ID
	proof := randomID() + randomID()
	attempt, err := a.BeginLogin(context.Background(), proof, true)
	must(t, err)
	if code := confirmLogin(t, server, attempt); code != 200 {
		t.Fatalf("confirmation: %d", code)
	}
	result, err := a.ClaimLogin(context.Background(), attempt.ID, proof)
	must(t, err)
	if result.State != "ready" || result.Session.User.ID != original || result.Session.User.Kind != "registered" {
		t.Fatal("upgrade replaced account")
	}
	var current model.RoomManagement
	must(t, a.Call(context.Background(), "GET", "/v2/rooms/"+room.Room.ID+"/manage", nil, &current))
	if current.Room.OwnerUserID != original || len(current.Members) != 1 {
		t.Fatal("upgrade lost room")
	}
	_, err = a.ClaimLogin(context.Background(), attempt.ID, proof)
	statusError(t, err, 410)
	proof = randomID() + randomID()
	attempt, err = b.BeginLogin(context.Background(), proof, true)
	must(t, err)
	if confirmLogin(t, server, attempt) != 409 {
		t.Fatal("conflict confirmation failed")
	}
	result, err = b.ClaimLogin(context.Background(), attempt.ID, proof)
	must(t, err)
	if result.State != "failed" || result.Code != "account_identity_conflict" {
		t.Fatal("existing OIDC identity merged accounts")
	}
	u, err := s.Account(context.Background(), b.Identity.ID())
	must(t, err)
	if u.Kind != "guest" {
		t.Fatal("conflicted guest upgraded")
	}
}

func TestOIDCRejectsInvalidTokens(t *testing.T) {
	for _, bad := range []string{"nonce", "aud", "issuer", "expired", "azp", "multi-aud"} {
		t.Run(bad, func(t *testing.T) {
			s, server := oidcFixture(t, bad)
			a := user(t, server, "guest")
			attempt, err := a.BeginLogin(context.Background(), randomID()+randomID(), true)
			must(t, err)
			if confirmLogin(t, server, attempt) != 403 {
				t.Fatal("invalid OIDC token accepted")
			}
			u, err := s.Account(context.Background(), a.Identity.ID())
			must(t, err)
			if u.Kind != "guest" {
				t.Fatal("failed login changed account")
			}
		})
	}
}

func TestAccountDeviceLoginOwnershipBanAndLogout(t *testing.T) {
	s, server := oidcFixture(t, "")
	a := user(t, server, "host")
	room := create(t, a)
	ctx := context.Background()
	proof := randomID() + randomID()
	attempt, err := a.BeginLogin(ctx, proof, true)
	must(t, err)
	if confirmLogin(t, server, attempt) != 200 {
		t.Fatal("link failed")
	}
	_, err = a.ClaimLogin(ctx, attempt.ID, proof)
	must(t, err)
	i, err := device.NewIdentity(server.URL, "second computer")
	must(t, err)
	second := client.NewAPI(i)
	proof = randomID() + randomID()
	attempt, err = second.BeginLogin(ctx, proof, false)
	must(t, err)
	if confirmLogin(t, server, attempt) != 200 {
		t.Fatal("login failed")
	}
	result, err := second.ClaimLogin(ctx, attempt.ID, proof)
	must(t, err)
	if result.Session.User.ID != a.Account().ID {
		t.Fatal("new device created another account")
	}
	var management model.RoomManagement
	must(t, second.Call(ctx, "GET", "/v2/rooms/"+room.Room.ID+"/manage", nil, &management))
	statusError(t, second.Call(ctx, "POST", "/v2/rooms/join", model.JoinRequest{Code: room.Invitation.Code}, nil), 409)
	old := lease(t, a, room.Room.ID)
	must(t, second.Call(ctx, "POST", "/v2/me/takeover", model.TakeoverRequest{RoomID: room.Room.ID, DeviceID: a.Identity.ID(), ExpectedRevision: roomRevision(t, s, room.Room.ID)}, nil))
	var revoked bool
	must(t, s.Pool.QueryRow(ctx, `SELECT revoked FROM certificates WHERE fingerprint=$1`, old.Fingerprint).Scan(&revoked))
	if !revoked {
		t.Fatal("takeover retained old certificate")
	}
	join(t, second, room)
	other := user(t, server, "other owner")
	otherRoom := create(t, other)
	must(t, second.Call(ctx, "POST", "/v2/rooms/"+room.Room.ID+"/leave", model.MemberRequest{}, nil))
	join(t, second, otherRoom)
	must(t, other.Call(ctx, "POST", "/v2/rooms/"+otherRoom.Room.ID+"/kick", model.MemberRequest{ExpectedRevision: roomRevision(t, s, otherRoom.Room.ID), DeviceID: second.Identity.ID()}, nil))
	statusError(t, a.Call(ctx, "POST", "/v2/rooms/join", model.JoinRequest{Code: otherRoom.Invitation.Code}, nil), 403)
	must(t, second.Call(ctx, "POST", "/v2/me/logout", struct{}{}, nil))
	statusError(t, client.NewAPI(i).Authenticate(ctx), 403)
}

func TestAdminUserRevocationAndOIDCSecretBoundary(t *testing.T) {
	s, admin := newAdmin(t)
	a := user(t, admin.server, "guest")
	room := create(t, a)
	cert := lease(t, a, room.Room.ID)
	id := a.Account().ID
	code, b := admin.request("GET", "/users?q=guest", nil, false, false)
	if code != 200 || !strings.Contains(string(b), id) {
		t.Fatalf("user search: %d", code)
	}
	code, b = admin.request("GET", "/users/"+id, nil, false, false)
	if code != 200 || strings.Contains(string(b), "token_hash") {
		t.Fatal("user detail leaked session credentials")
	}
	code, _ = admin.request("POST", "/users/"+id+"/actions", model.UserAction{Action: "disable", Reason: "test abuse"}, false, true)
	if code != 403 {
		t.Fatal("user mutation lacked CSRF protection")
	}
	code, _ = admin.request("POST", "/users/"+id+"/actions", model.UserAction{Action: "disable", Reason: "test abuse"}, true, true)
	if code != 200 {
		t.Fatalf("disable: %d", code)
	}
	statusError(t, a.Call(context.Background(), "POST", "/v2/rooms", model.RoomRequest{ExpectedGameRevision: 1, Name: "blocked", Game: "custom"}, nil), 403)
	var closed, revoked bool
	must(t, s.Pool.QueryRow(context.Background(), `SELECT r.closed,c.revoked FROM rooms r JOIN certificates c ON c.room_id=r.id WHERE c.fingerprint=$1`, cert.Fingerprint).Scan(&closed, &revoked))
	if !closed || !revoked {
		t.Fatal("user disable retained authorization")
	}
	code, _ = admin.request("PUT", "/oidc", model.OIDCSettings{Issuer: "https://identity.example", ClientID: "test", ClientSecret: "never-return-secret", Enabled: true}, true, true)
	if code != 200 {
		t.Fatalf("OIDC settings: %d", code)
	}
	code, b = admin.request("GET", "/oidc", nil, false, false)
	if code != 200 || strings.Contains(string(b), "never-return-secret") {
		t.Fatal("OIDC secret returned")
	}
}

func TestIdempotencyChecksRevocationInsideTransaction(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	a := user(t, server, "guest")
	ctx := context.Background()
	key := randomID()
	id := a.Identity.ID()
	check := func(tx pgx.Tx) error { _, e := playerUser(ctx, tx, id); return e }
	_, err := s.mutateChecked(ctx, id, key, "request", check, func(tx pgx.Tx) (any, error) { return map[string]bool{"ok": true}, nil })
	must(t, err)
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE users SET state='disabled' WHERE id=$1`, a.Account().ID)
		return e
	}))
	_, err = s.mutateChecked(ctx, id, key, "request", check, func(tx pgx.Tx) (any, error) { t.Fatal("replayed operation ran"); return nil, nil })
	if !model.IsCode(err, "account_disabled") {
		t.Fatalf("cached response bypassed authorization: %v", err)
	}
}

func TestCachedPlayerLeaseCannotRestoreRevokedMembership(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	ctx := context.Background()
	a := user(t, server, "owner")
	room := create(t, a)
	pub := ed25519.PrivateKey(a.Identity.PrivateKey).Public().(ed25519.PublicKey)
	c, err := s.Challenge(ctx, model.ChallengeRequest{DeviceID: a.Identity.ID(), Name: a.Identity.Name, PublicKey: pub})
	must(t, err)
	session, err := s.Verify(ctx, model.VerifyRequest{ID: c.ID, Signature: ed25519.Sign(a.Identity.PrivateKey, append([]byte("nodelane-auth-v2:player:"+c.ID+":"), c.Nonce...))})
	must(t, err)
	_, tunnel, err := pki.TunnelKey()
	must(t, err)
	body, err := json.Marshal(model.LeaseRequest{PublicKey: tunnel})
	must(t, err)
	key := randomID()
	deadline := time.Now().UTC().Add(50 * time.Minute)
	request := func() int {
		req, err := http.NewRequest("POST", server.URL+"/v2/rooms/"+room.Room.ID+"/lease", bytes.NewReader(body))
		must(t, err)
		req.Header.Set("Authorization", "Bearer "+session.Token)
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set(model.ContractHeader, model.Contract)
		req.Header.Set(model.DeadlineHeader, deadline.Format(time.RFC3339Nano))
		resp, err := server.Client().Do(req)
		must(t, err)
		resp.Body.Close()
		return resp.StatusCode
	}
	if request() != 200 || request() != 200 {
		t.Fatal("valid lease retry rejected")
	}
	must(t, a.Call(ctx, "POST", "/v2/rooms/"+room.Room.ID+"/leave", model.MemberRequest{}, nil))
	if request() != 403 {
		t.Fatal("cached lease survived leaving")
	}
	join(t, a, room)
	if request() != 409 {
		t.Fatal("rejoining recovered the revoked certificate")
	}
}

func TestOIDCClaimRequiresOriginalProofAndDevice(t *testing.T) {
	_, server := oidcFixture(t, "")
	a := user(t, server, "guest")
	proof := randomID() + randomID()
	attempt, err := a.BeginLogin(context.Background(), proof, true)
	must(t, err)
	b := user(t, server, "other device")
	_, err = b.ClaimLogin(context.Background(), attempt.ID, proof)
	statusError(t, err, 401)
	_, err = a.ClaimLogin(context.Background(), attempt.ID, randomID()+randomID())
	statusError(t, err, 401)
	result, err := a.ClaimLogin(context.Background(), attempt.ID, proof)
	must(t, err)
	if result.State != "pending" || result.Session != nil {
		t.Fatal("unauthorized claim changed pending login")
	}
}
