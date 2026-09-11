package control

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/theupdateframework/go-tuf/v2/metadata"
)

func updateFixture(t *testing.T, s *Store) model.UpdateRelease {
	t.Helper()
	ctx := context.Background()
	_, key, _ := ed25519.GenerateKey(nil)
	signer, e := signature.LoadSigner(key, crypto.Hash(0))
	must(t, e)
	pub, e := metadata.KeyFromPublicKey(key.Public())
	must(t, e)
	root := metadata.Root(time.Now().Add(time.Hour))
	for _, role := range []string{"root", "timestamp", "snapshot", "targets"} {
		must(t, root.Signed.AddKey(pub, role))
	}
	_, e = root.Sign(signer)
	must(t, e)
	rb, e := root.ToBytes(false)
	must(t, e)
	rootPath := filepath.Join(t.TempDir(), "root.json")
	must(t, os.WriteFile(rootPath, rb, 0600))
	t.Setenv("NODELANE_UPDATE_ROOT", rootPath)
	targets := metadata.Targets(time.Now().Add(time.Hour))
	tf, e := metadata.TargetFile().FromBytes("nlroom-9.0.0.exe", []byte("fixture"), "sha256")
	must(t, e)
	custom := json.RawMessage(`{"version":"9.0.0","os":"windows","arch":"amd64"}`)
	tf.Custom = &custom
	targets.Signed.Targets["nlroom-9.0.0.exe"] = tf
	_, e = targets.Sign(signer)
	must(t, e)
	tb, e := targets.ToBytes(false)
	must(t, e)
	snapshot := metadata.Snapshot(time.Now().Add(time.Hour))
	snapshot.Signed.Meta["targets.json"] = metadata.MetaFile(1)
	_, e = snapshot.Sign(signer)
	must(t, e)
	sb, e := snapshot.ToBytes(false)
	must(t, e)
	timestamp := metadata.Timestamp(time.Now().Add(time.Hour))
	timestamp.Signed.Meta["snapshot.json"] = metadata.MetaFile(1)
	_, e = timestamp.Sign(signer)
	must(t, e)
	ts, e := timestamp.ToBytes(false)
	must(t, e)
	repo := model.UpdateRepository{Metadata: map[string]json.RawMessage{"root.json": rb, "1.root.json": rb, "targets.json": tb, "snapshot.json": sb, "timestamp.json": ts}}
	var result model.UpdateRelease
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		if _, e := saveRepository(ctx, tx, "admin", repo); e != nil {
			return e
		}
		v, e := saveRelease(ctx, tx, "admin", model.UpdateRelease{UpdateArtifact: model.UpdateArtifact{Target: "nlroom-9.0.0.exe"}, State: "draft"})
		if e != nil {
			return e
		}
		result = v.(model.UpdateRelease)
		source, e := saveSource(ctx, tx, "admin", model.UpdateSource{Name: "mirror", Kind: "https", PublicURL: "https://updates.example.test", Enabled: true})
		if e != nil {
			return e
		}
		src := source.(model.UpdateSource)
		if _, e = tx.Exec(ctx, "INSERT INTO update_replicas(release_id,source_id,verified_at,source_revision) VALUES($1,$2,now(),$3)", result.ID, src.ID, src.Revision); e != nil {
			return e
		}
		result.State = "published"
		v, e = saveRelease(ctx, tx, "admin", result)
		if e != nil {
			return e
		}
		result = v.(model.UpdateRelease)
		return nil
	}))
	return result
}

func TestForcedUpdateRevokesExistingAuthorization(t *testing.T) {
	s, ca := database(t)
	release := updateFixture(t, s)
	server := apiServer(t, s, ca)
	a := user(t, server, "old-client")
	ctx := context.Background()
	report := model.ClientReport{Version: "0.2.1", OS: "windows", Arch: "amd64", State: "idle"}
	must(t, a.Call(ctx, "POST", "/v2/client/report", report, nil))
	room := create(t, a)
	certificate := lease(t, a, room.Room.ID)
	future := time.Now().Add(time.Hour)
	p := model.UpdatePolicy{OS: "windows", Arch: "amd64", ReleaseID: release.ID, MinimumVersion: "9.0.0", EffectiveAt: &future}
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		v, e := savePolicy(ctx, tx, "admin", p)
		if e == nil {
			p = v.(model.UpdatePolicy)
		}
		return e
	}))
	heartbeatCtx := client.WithOperation(ctx, randomID(), time.Now().UTC().Add(50*time.Minute))
	must(t, a.Call(heartbeatCtx, "POST", "/v2/rooms/"+room.Room.ID+"/heartbeat", model.HeartbeatRequest{LANVersion: model.LANVersion}, nil))
	past := time.Now().Add(-time.Minute)
	p.EffectiveAt = &past
	must(t, s.Write(ctx, func(tx pgx.Tx) error { _, e := savePolicy(ctx, tx, "admin", p); return e }))
	var revoked, active bool
	must(t, s.Pool.QueryRow(ctx, "SELECT revoked FROM certificates WHERE fingerprint=$1", certificate.Fingerprint).Scan(&revoked))
	must(t, s.Pool.QueryRow(ctx, "SELECT active FROM members WHERE device_id=$1 AND room_id=$2", a.Identity.ID(), room.Room.ID).Scan(&active))
	if !revoked || active {
		t.Fatal("old authorization survives mandatory update")
	}
	for _, tc := range []struct {
		path string
		body any
	}{
		{"/v2/rooms", model.RoomRequest{ExpectedGameRevision: 1, Name: "denied", Game: "custom"}},
		{"/v2/rooms/join", model.JoinRequest{Code: room.Invitation.Code}},
		{"/v2/rooms/" + room.Room.ID + "/join", model.MemberRequest{ExpectedRevision: room.Room.Revision}},
		{"/v2/rooms/" + room.Room.ID + "/lease", model.LeaseRequest{}},
		{"/v2/rooms/" + room.Room.ID + "/heartbeat", model.HeartbeatRequest{LANVersion: model.LANVersion}},
	} {
		if err := a.Call(ctx, "POST", tc.path, tc.body, nil); !model.IsCode(err, "client_update_required") {
			t.Fatalf("%s bypassed version check: %v", tc.path, err)
		}
	}
	if err := a.Call(heartbeatCtx, "POST", "/v2/rooms/"+room.Room.ID+"/heartbeat", model.HeartbeatRequest{LANVersion: model.LANVersion}, nil); !model.IsCode(err, "client_update_required") {
		t.Fatalf("cached heartbeat bypassed current version check: %v", err)
	}
	must(t, a.Call(ctx, "GET", "/v2/me", nil, &model.User{}))
	report.Version = "9.0.0"
	report.State = "succeeded"
	report.ReleaseID = release.ID
	must(t, a.Call(ctx, "POST", "/v2/client/report", report, nil))
	_ = create(t, a)
}

func TestUpdatePolicyRevisionAndSourceAvailability(t *testing.T) {
	s, _ := database(t)
	release := updateFixture(t, s)
	ctx := context.Background()
	p := model.UpdatePolicy{OS: "windows", Arch: "amd64", ReleaseID: release.ID}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.Write(ctx, func(tx pgx.Tx) error { _, e := savePolicy(ctx, tx, "admin", p); return e })
		}()
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("stale concurrent policy accepted")
	}
	err := s.Write(ctx, func(tx pgx.Tx) error {
		src, e := readSource(ctx, tx, release.Sources[0], false)
		if e != nil {
			return e
		}
		src.Enabled = false
		_, e = saveSource(ctx, tx, "admin", src)
		return e
	})
	if !model.IsCode(err, "update_last_source_required") {
		t.Fatal("disabled last active source", err)
	}
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		src, e := readSource(ctx, tx, release.Sources[0], false)
		if e != nil {
			return e
		}
		src.Priority = 2
		if _, e = saveSource(ctx, tx, "admin", src); e != nil {
			return e
		}
		r, e := readRelease(ctx, tx, release.ID)
		if e != nil {
			return e
		}
		if len(r.Sources) != 1 {
			t.Fatal("priority edit invalidated verified bytes")
		}
		_, e = savePolicy(ctx, tx, "admin", model.UpdatePolicy{OS: p.OS, Arch: p.Arch, Revision: 1})
		return e
	}))
	if e := s.Write(ctx, func(tx pgx.Tx) error { _, e := savePolicy(ctx, tx, "admin", p); return e }); !errors.Is(e, ErrConflict) {
		t.Fatal("deleted policy accepted stale create", e)
	}
}

func TestUpdateManagementAuthenticationAndOverview(t *testing.T) {
	s, a := newAdmin(t)
	_ = updateFixture(t, s)
	status, _ := a.request("PUT", "/updates/sources", model.UpdateSource{Name: "mirror", Kind: "https", PublicURL: "https://example.com", Enabled: true}, true, false)
	if status != 403 {
		t.Fatal("source update accepted without CSRF", status)
	}
	status, b := a.request("GET", "/updates", nil, false, false)
	if status != 200 {
		t.Fatalf("overview %d %s", status, b)
	}
	var view model.UpdateOverview
	must(t, json.Unmarshal(b, &view))
	if len(view.Releases) != 1 || len(view.Sources) != 1 || view.RepositoryRevision != 1 {
		t.Fatalf("overview %+v", view)
	}
	if bytes.Contains(b, []byte("secret_key")) || bytes.Contains(b, []byte("access_key")) {
		t.Fatal("overview included credentials")
	}
}

func TestUpdateRepositoryRequestLimit(t *testing.T) {
	_, a := newAdmin(t)
	body := map[string]string{"unexpected": strings.Repeat("x", 70000)}
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/updates/repository", http.StatusBadRequest},
		{"/updates/sources", http.StatusRequestEntityTooLarge},
	} {
		status, _ := a.request("PUT", tc.path, body, true, true)
		if status != tc.status {
			t.Fatalf("%s: status %d, want %d", tc.path, status, tc.status)
		}
	}
}

func TestUpdateStorageSecretsEncryptedAndWriteOnly(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	t.Setenv("NODELANE_UPDATE_STORAGE_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		v, e := saveSource(ctx, tx, "admin", model.UpdateSource{Name: "R2", Kind: "r2", Endpoint: "https://account.r2.cloudflarestorage.com", Bucket: "releases", AccessKey: "test-access", SecretKey: "test-secret", Enabled: true})
		if e != nil {
			return e
		}
		src := v.(model.UpdateSource)
		if src.AccessKey != "" || src.SecretKey != "" || !src.HasCredentials {
			t.Fatal("write response exposed credentials")
		}
		got, e := readSource(ctx, tx, src.ID, true)
		if e != nil {
			return e
		}
		if got.SecretKey != "test-secret" {
			t.Fatal("credentials lost")
		}
		var b []byte
		if e = tx.QueryRow(ctx, "SELECT config::text FROM update_sources WHERE id=$1", src.ID).Scan(&b); e != nil {
			return e
		}
		if strings.Contains(string(b), "test-secret") {
			t.Fatal("plaintext secret")
		}
		return nil
	}))
}

func TestS3CompatibleClientAndPrivateDownloadURL(t *testing.T) {
	var heads, puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("missing SigV4 authorization")
		}
		switch r.Method {
		case "HEAD":
			heads++
			if r.URL.Path != "/packages" {
				t.Error("wrong bucket", r.URL.Path)
			}
		case "PUT":
			puts++
			if r.URL.Path != "/packages/releases/fixture.exe" || r.Header.Get("If-None-Match") != "*" {
				t.Error("nonimmutable upload")
			}
			b, e := io.ReadAll(r.Body)
			if e != nil || string(b) != "signed bytes" {
				t.Error("wrong package bytes")
			}
		default:
			t.Error("unexpected storage request", r.Method)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	// Local HTTP is used only by this SDK protocol fixture; saved production
	// configurations and package download paths require HTTPS.
	src := model.UpdateSource{Endpoint: srv.URL, Bucket: "packages", Region: "auto", Prefix: "releases", PathStyle: true, AccessKey: "fixture-access", SecretKey: "fixture-secret"}
	c := sourceClient(src)
	ctx := context.Background()
	_, e := c.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(src.Bucket)})
	must(t, e)
	_, e = c.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(src.Bucket), Key: aws.String("releases/fixture.exe"), Body: strings.NewReader("signed bytes"), ContentLength: aws.Int64(12), IfNoneMatch: aws.String("*")})
	must(t, e)
	u, e := sourceURL(ctx, src, "fixture.exe")
	must(t, e)
	parsed, e := url.Parse(u)
	must(t, e)
	if heads != 1 || puts != 1 || parsed.Query().Get("X-Amz-Expires") != "7200" || parsed.Query().Get("X-Amz-Signature") == "" || strings.Contains(u, src.SecretKey) {
		t.Fatal("invalid S3 private download behavior")
	}
}
