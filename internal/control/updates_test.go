package control

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

func updateRepositoryFixture(t *testing.T, timestampExpires time.Time, additional ...model.UpdateArtifact) model.UpdateRepository {
	t.Helper()
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
	for _, a := range append([]model.UpdateArtifact{{Version: "9.0.0", OS: "windows", Arch: "amd64", Target: "nlroom-9.0.0.exe"}}, additional...) {
		tf, e := metadata.TargetFile().FromBytes(a.Target, []byte("fixture"), "sha256")
		must(t, e)
		b, e := json.Marshal(a)
		must(t, e)
		custom := json.RawMessage(b)
		tf.Custom = &custom
		targets.Signed.Targets[a.Target] = tf
	}
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
	timestamp := metadata.Timestamp(timestampExpires)
	timestamp.Signed.Meta["snapshot.json"] = metadata.MetaFile(1)
	_, e = timestamp.Sign(signer)
	must(t, e)
	ts, e := timestamp.ToBytes(false)
	must(t, e)
	return model.UpdateRepository{Metadata: map[string]json.RawMessage{"root.json": rb, "1.root.json": rb, "targets.json": tb, "snapshot.json": sb, "timestamp.json": ts}}
}

func updateFixture(t *testing.T, s *Store, additional ...model.UpdateArtifact) model.UpdateRelease {
	t.Helper()
	ctx := context.Background()
	repo := updateRepositoryFixture(t, time.Now().Add(time.Hour), additional...)
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
	release := updateFixture(t, s)
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
	got := view.Releases[0]
	if got.ID != release.ID || got.UpdateArtifact != release.UpdateArtifact || got.Revision != release.Revision || got.State != "published" || !got.CreatedAt.Equal(release.CreatedAt) || len(got.Sources) != 1 || got.Sources[0] != release.Sources[0] {
		t.Fatal("overview lost published release details", got)
	}
	if bytes.Contains(b, []byte("secret_key")) || bytes.Contains(b, []byte("access_key")) {
		t.Fatal("overview included credentials")
	}
}

func TestUpdateCatalogReadsDoNotWaitForWrites(t *testing.T) {
	s, _ := database(t)
	release := updateFixture(t, s)
	ctx := context.Background()
	tx, err := s.Pool.Begin(ctx)
	must(t, err)
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(1313817669)")
	must(t, err)
	_, err = tx.Exec(ctx, "UPDATE update_releases SET state='paused' WHERE id=$1", release.ID)
	must(t, err)
	t.Run("public catalog", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		catalog, err := s.downloads(ctx)
		must(t, err)
		if len(catalog.Releases) != 1 || catalog.Releases[0].ID != release.ID {
			t.Fatal("catalog did not retain the committed publication")
		}
	})
	t.Run("admin overview", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		view, err := s.updateOverview(ctx, "")
		must(t, err)
		if len(view.Releases) != 1 || view.Releases[0].State != "published" || len(view.Releases[0].Sources) != 1 {
			t.Fatal("overview did not retain the committed publication")
		}
	})
	// URL issuance must still serialize with publication and source changes.
	waitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if _, err := s.downloadRelease(waitCtx, release.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("URL issuance bypassed an in-flight publication change: %v", err)
	}
	must(t, tx.Commit(ctx))
	catalog, err := s.downloads(ctx)
	must(t, err)
	if len(catalog.Releases) != 0 {
		t.Fatal("new catalog retained a paused publication")
	}
	if _, err := s.downloadRelease(ctx, release.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("paused publication remained downloadable: %v", err)
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

func TestPublicDownloadsContractAndSetup(t *testing.T) {
	for name, handler := range map[string]http.Handler{"server": (&Server{}).Handler(), "setup": (&Deployment{}).Handler()} {
		for _, path := range []string{"/v2/downloads", "/v2/downloads/" + strings.Repeat("a", 32)} {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, contractRequest("GET", path, nil))
			var result model.Result
			must(t, json.Unmarshal(w.Body.Bytes(), &result))
			if w.Code != 503 || result.Code != "system_unavailable" || result.Contract != model.Contract || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s %s: %d %s", name, path, w.Code, w.Body.String())
			}
			w = httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			must(t, json.Unmarshal(w.Body.Bytes(), &result))
			if result.Code != "api_contract_unsupported" {
				t.Fatalf("%s accepted missing contract", name)
			}
		}
	}
}

func TestPublicDownloadsCatalogAndLinks(t *testing.T) {
	s, ca := database(t)
	release := updateFixture(t, s,
		model.UpdateArtifact{Version: "9.2.0", OS: "windows", Arch: "amd64", Target: "nlroom-9.2.0.exe"},
		model.UpdateArtifact{Version: "9.10.0", OS: "windows", Arch: "amd64", Target: "nlroom-9.10.0.exe"})
	ctx := context.Background()
	catalog, err := s.downloads(ctx)
	must(t, err)
	if len(catalog.Releases) != 1 || catalog.Releases[0].Recommended || catalog.ServerTime.IsZero() {
		t.Fatal("published installer requires an update policy", catalog)
	}
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		for _, target := range []string{"nlroom-9.2.0.exe", "nlroom-9.10.0.exe"} {
			v, err := saveRelease(ctx, tx, "admin", model.UpdateRelease{UpdateArtifact: model.UpdateArtifact{Target: target}, State: "draft"})
			if err != nil {
				return err
			}
			r := v.(model.UpdateRelease)
			if _, err = tx.Exec(ctx, "INSERT INTO update_replicas(release_id,source_id,verified_at,source_revision) SELECT $1,id,now(),revision FROM update_sources WHERE id=$2", r.ID, release.Sources[0]); err != nil {
				return err
			}
			r.State = "published"
			if _, err = saveRelease(ctx, tx, "admin", r); err != nil {
				return err
			}
		}
		_, err := savePolicy(ctx, tx, "admin", model.UpdatePolicy{OS: release.OS, Arch: release.Arch, ReleaseID: release.ID})
		return err
	}))
	handler := (&Server{Store: s, CA: ca}).Handler()
	get := func(path string, out any) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, contractRequest("GET", path, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		b := responseData(t, w.Body.Bytes())
		must(t, json.Unmarshal(b, out))
		return b
	}
	b := get("/v2/downloads", &catalog)
	if len(catalog.Releases) != 3 || catalog.Releases[0].Version != "9.0.0" || !catalog.Releases[0].Recommended || catalog.Releases[1].Version != "9.10.0" || catalog.Releases[2].Version != "9.2.0" {
		t.Fatal("wrong recommendation or numeric version order", catalog)
	}
	for _, forbidden := range []string{"sources", "revision", "state", "metadata", "devices", "secret_key", "access_key", "https://"} {
		if bytes.Contains(b, []byte(forbidden)) {
			t.Fatalf("catalog exposes %s", forbidden)
		}
	}
	var links model.DownloadLinks
	get("/v2/downloads/"+release.ID, &links)
	if links.Release.ID != release.ID || len(links.URLs) != 1 || links.URLs[0] != "https://updates.example.test/nlroom-9.0.0.exe" || links.Release.SHA256 != release.SHA256 {
		t.Fatal("wrong installer or source URL", links)
	}
	t.Setenv("NODELANE_UPDATE_STORAGE_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		v, err := saveSource(ctx, tx, "admin", model.UpdateSource{Name: "private", Kind: "s3", Endpoint: "https://objects.example.test", Bucket: "packages", AccessKey: "fixture-access", SecretKey: "fixture-secret", Enabled: true, Priority: 1})
		if err != nil {
			return err
		}
		source := v.(model.UpdateSource)
		_, err = tx.Exec(ctx, "INSERT INTO update_replicas(release_id,source_id,verified_at,source_revision) VALUES($1,$2,now(),$3)", release.ID, source.ID, source.Revision)
		return err
	}))
	b = get("/v2/downloads", &catalog)
	if bytes.Contains(b, []byte("objects.example.test")) || bytes.Contains(b, []byte("fixture-access")) {
		t.Fatal("private storage configuration appeared in catalog")
	}
	get("/v2/downloads/"+release.ID, &links)
	if len(links.URLs) != 2 || links.URLs[0] != "https://updates.example.test/nlroom-9.0.0.exe" {
		t.Fatal("download source priority lost")
	}
	privateURL, err := url.Parse(links.URLs[1])
	must(t, err)
	if privateURL.Query().Get("X-Amz-Expires") != "7200" || privateURL.Query().Get("X-Amz-Signature") == "" || strings.Contains(links.URLs[1], "fixture-secret") {
		t.Fatal("private installer URL was not safely presigned")
	}
	for _, id := range []string{"invalid", randomID()} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, contractRequest("GET", "/v2/downloads/"+id, nil))
		if w.Code != 404 {
			t.Fatalf("invalid installer %s: %d", id, w.Code)
		}
	}
	for range 120 {
		_ = s.Rate(ctx, "downloads:192.0.2.1", 120, time.Minute)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, contractRequest("GET", "/v2/downloads/"+release.ID, nil))
	if w.Code != 429 || w.Header().Get("Retry-After") != "60" {
		t.Fatalf("downloads did not share IP rate limit: %d", w.Code)
	}
}

func TestPublicDownloadsRecheckPublicationAndTrust(t *testing.T) {
	for _, change := range []string{"draft", "paused", "withdrawn", "source-disabled", "source-revised", "replica-missing", "metadata-expired", "artifact-mismatch"} {
		t.Run(change, func(t *testing.T) {
			s, ca := database(t)
			release := updateFixture(t, s)
			ctx := context.Background()
			links, err := s.downloadRelease(ctx, release.ID)
			must(t, err)
			if len(links.URLs) != 1 {
				t.Fatal("published installer not downloadable")
			}
			must(t, s.Write(ctx, func(tx pgx.Tx) error {
				switch change {
				case "draft", "paused", "withdrawn":
					release.State = change
					_, err = saveRelease(ctx, tx, "admin", release)
				case "source-disabled", "source-revised":
					var source model.UpdateSource
					source, err = readSource(ctx, tx, release.Sources[0], false)
					if err != nil {
						return err
					}
					if change == "source-disabled" {
						source.Enabled = false
					} else {
						source.PublicURL = "https://replacement.example.test"
					}
					_, err = saveSource(ctx, tx, "admin", source)
				case "replica-missing":
					_, err = tx.Exec(ctx, "DELETE FROM update_replicas WHERE release_id=$1", release.ID)
				case "metadata-expired":
					repo := updateRepositoryFixture(t, time.Now().Add(-time.Minute))
					var b []byte
					b, err = json.Marshal(repo.Metadata)
					if err == nil {
						_, err = tx.Exec(ctx, "UPDATE update_repository SET metadata=$1 WHERE id=1", b)
					}
				case "artifact-mismatch":
					_, err = tx.Exec(ctx, "UPDATE update_releases SET artifact=jsonb_set(artifact,'{sha256}',to_jsonb($2::text)) WHERE id=$1", release.ID, strings.Repeat("0", 64))
				}
				return err
			}))
			catalog, err := s.downloads(ctx)
			must(t, err)
			if len(catalog.Releases) != 0 {
				t.Fatal("unavailable installer remained public", catalog)
			}
			w := httptest.NewRecorder()
			(&Server{Store: s, CA: ca}).Handler().ServeHTTP(w, contractRequest("GET", "/v2/downloads/"+release.ID, nil))
			var result model.Result
			must(t, json.Unmarshal(w.Body.Bytes(), &result))
			if w.Code != 404 || result.Code != "resource_not_found" || string(result.Data) != "null" {
				t.Fatalf("stale download escaped recheck: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestPublicDownloadsPlatformCapacity(t *testing.T) {
	s, _ := database(t)
	ctx := context.Background()
	artifacts := []model.UpdateArtifact{}
	for _, platform := range []struct {
		os, arch string
		count    int
	}{{"linux", "amd64", 205}, {"linux", "arm64", 55}, {"windows", "amd64", 55}, {"windows", "arm64", 55}} {
		for version := 1; version <= platform.count; version++ {
			ext := ".exe"
			if platform.os == "linux" {
				ext = ".deb"
			}
			artifacts = append(artifacts, model.UpdateArtifact{Version: fmt.Sprintf("10.0.%d", version), OS: platform.os, Arch: platform.arch, Target: fmt.Sprintf("nlroom-%s-%s-%d%s", platform.os, platform.arch, version, ext)})
		}
	}
	// These signed targets have invalid stored hashes; they must not consume the
	// Windows ARM64 quota ahead of its lower-version, valid installers.
	for version := 1; version <= 60; version++ {
		artifacts = append(artifacts, model.UpdateArtifact{Version: fmt.Sprintf("99.0.%d", version), OS: "windows", Arch: "arm64", Target: fmt.Sprintf("mismatch-%d.exe", version)})
	}
	release := updateFixture(t, s, artifacts...)
	// Unsigned newer targets must also be filtered before applying the quota.
	for version := 1; version <= 60; version++ {
		artifacts = append(artifacts, model.UpdateArtifact{Version: fmt.Sprintf("99.0.%d", version), OS: "windows", Arch: "amd64", Target: fmt.Sprintf("unsigned-%d.exe", version)})
	}
	oldest := ""
	must(t, s.Write(ctx, func(tx pgx.Tx) error {
		for _, a := range artifacts {
			a.Size = int64(len("fixture"))
			a.SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("fixture")))
			if strings.HasPrefix(a.Target, "mismatch-") {
				a.SHA256 = strings.Repeat("0", 64)
			}
			b, err := json.Marshal(a)
			if err != nil {
				return err
			}
			id := randomID()
			if oldest == "" {
				oldest = id
			}
			if _, err = tx.Exec(ctx, "INSERT INTO update_releases(id,version,os,arch,artifact,notes,state) VALUES($1,$2,$3,$4,$5,'','published')", id, a.Version, a.OS, a.Arch, b); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "INSERT INTO update_replicas(release_id,source_id,verified_at,source_revision) SELECT $1,id,now(),revision FROM update_sources WHERE id=$2", id, release.Sources[0]); err != nil {
				return err
			}
		}
		_, err := savePolicy(ctx, tx, "admin", model.UpdatePolicy{OS: release.OS, Arch: release.Arch, ReleaseID: release.ID})
		return err
	}))
	catalog, err := s.downloads(ctx)
	must(t, err)
	if len(catalog.Releases) != 200 {
		t.Fatalf("catalog size %d, want 200", len(catalog.Releases))
	}
	counts := map[string]int{}
	previous := map[string]model.DownloadRelease{}
	foundRecommended := false
	for _, r := range catalog.Releases {
		platform := r.OS + "/" + r.Arch
		counts[platform]++
		if strings.HasPrefix(r.Target, "mismatch-") || strings.HasPrefix(r.Target, "unsigned-") {
			t.Fatal("untrusted artifact used a platform quota", r.Target)
		}
		if last, ok := previous[platform]; ok && (r.Recommended || !last.Recommended && model.CompareVersion(last.Version, r.Version) < 0) {
			t.Fatalf("wrong platform order: %s then %s", last.Version, r.Version)
		}
		previous[platform] = r
		if r.ID == release.ID {
			foundRecommended = r.Recommended && counts[platform] == 1
		}
	}
	for _, platform := range []string{"windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64"} {
		if counts[platform] != 50 {
			t.Fatalf("%s got %d entries, want 50", platform, counts[platform])
		}
	}
	if !foundRecommended {
		t.Fatal("busy Linux platform displaced the Windows recommendation")
	}
	links, err := s.downloadRelease(ctx, oldest)
	must(t, err)
	if links.Release.ID != oldest || len(links.URLs) != 1 {
		t.Fatal("valid installer outside catalog quota lost its download")
	}
}
