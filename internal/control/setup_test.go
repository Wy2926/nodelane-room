package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
	"github.com/nodelane/nodelane-room/internal/platform"
)

func setupDatabaseURL(t *testing.T, s *Store) string {
	t.Helper()
	var schema string
	must(t, s.Pool.QueryRow(context.Background(), "SELECT current_schema()").Scan(&schema))
	u, err := url.Parse(os.Getenv("NODELANE_TEST_DATABASE_URL"))
	must(t, err)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func setupRequest(t *testing.T, d *Deployment, in SetupRequest, origin string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(in)
	must(t, err)
	req := contractRequest("POST", "https://room.test/v2/admin/setup", bytes.NewReader(b))
	req.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	d.Handler().ServeHTTP(w, req)
	return w
}

func setupInstance(t *testing.T) (*Deployment, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("control deployment runs on Linux; private directory integration runs in Linux Docker")
	}
	d := &Deployment{StateDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var err error
	d.AdminPath, err = ConfigureAdminPath(d.StateDir, "")
	must(t, err)
	code, err := BootstrapCode(context.Background(), d.StateDir)
	must(t, err)
	return d, code
}

func createRequest(code, db string) SetupRequest {
	return SetupRequest{Mode: "create", Code: code, Username: "owner", Password: "correct test password", DatabaseURL: db, PublicURL: "room.test", Network: model.DefaultPool, Registry: "registry.test:5000", CAMode: "generate"}
}

func TestSetupWithoutDatabaseAndCodeSecurity(t *testing.T) {
	d, code := setupInstance(t)
	for path, status := range map[string]int{"/": 404, "/admin": 404, d.AdminPath: 200, "/healthz": 200, "/readyz": 503, "/v2/rooms": 503, "/v2/admin/setup": 200} {
		w := httptest.NewRecorder()
		d.Handler().ServeHTTP(w, contractRequest("GET", path, nil))
		if w.Code != status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	in := createRequest(code, "postgres://user:secret@127.0.0.1:1/test")
	for _, origin := range []string{"", "https://evil.test"} {
		if w := setupRequest(t, d, in, origin); w.Code != 403 {
			t.Fatalf("origin accepted: %d", w.Code)
		}
	}
	rotated, err := BootstrapCode(context.Background(), d.StateDir)
	must(t, err)
	if d.validCode(code) || !d.validCode(rotated) {
		t.Fatal("console code rotation failed")
	}
	if w := setupRequest(t, d, in, "https://room.test"); w.Code != 403 {
		t.Fatalf("old code accepted: %d", w.Code)
	}
	b, err := json.Marshal(setupCode{Hash: hash(rotated), Expires: time.Now().Add(-time.Second)})
	must(t, err)
	must(t, platform.SavePrivateFile(filepath.Join(d.StateDir, "bootstrap.bin"), b, true))
	if d.validCode(rotated) {
		t.Fatal("expired code accepted")
	}
	if _, err := DatabaseURL(d.StateDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unauthorized request persisted a database locator")
	}
}

func TestSetupCreatesAndConnectsIndependentInstances(t *testing.T) {
	s, _ := database(t)
	db := setupDatabaseURL(t, s)
	// Exercise transactional schema creation on a genuinely empty test schema.
	var schema string
	must(t, s.Pool.QueryRow(context.Background(), "SELECT current_schema()").Scan(&schema))
	_, err := s.Pool.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE; CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	must(t, err)
	d, code := setupInstance(t)
	in := createRequest(code, db)
	if w := setupRequest(t, d, in, "https://room.test"); w.Code != 200 {
		t.Fatalf("setup: %d %s", w.Code, w.Body.String())
	}
	stored, err := DatabaseURL(d.StateDir)
	must(t, err)
	if stored != db {
		t.Fatal("database locator not persisted")
	}
	api, err := LoadDeployment(context.Background(), db)
	must(t, err)
	defer api.Store.Pool.Close()
	if api.PublicURL != "https://room.test" || api.Registry != "registry.test:5000" || api.Store.Network.String() != in.Network {
		t.Fatal("stored deployment configuration not loaded")
	}
	var savedDB, savedKey string
	must(t, s.Pool.QueryRow(context.Background(), "SELECT database_url,ca_key FROM deployment").Scan(&savedDB, &savedKey))
	if savedDB != db || savedKey != api.CA.SigningPEM() {
		t.Fatal("database configuration or CA not persisted")
	}
	if w := setupRequest(t, d, in, "https://room.test"); w.Code != 409 {
		t.Fatalf("setup replay accepted: %d", w.Code)
	}
	if _, err = BootstrapCode(context.Background(), d.StateDir); !model.IsCode(err, "setup_already_configured") {
		t.Fatalf("bootstrap reopened: %v", err)
	}
	second, code2 := setupInstance(t)
	connect := SetupRequest{Mode: "connect", Code: code2, DatabaseURL: db, Username: in.Username, Password: "incorrect password"}
	if w := setupRequest(t, second, connect, "https://room.test"); w.Code != 401 {
		t.Fatalf("connected with wrong credentials: %d", w.Code)
	}
	if _, err = DatabaseURL(second.StateDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed authentication bound instance")
	}
	connect.Password = in.Password
	if w := setupRequest(t, second, connect, "https://room.test"); w.Code != 200 {
		t.Fatalf("connect: %d %s", w.Code, w.Body.String())
	}
	// Simulate restart and concurrent serving with separate local directories.
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for _, instance := range []*Deployment{d, second} {
		wg.Add(1)
		go func() { defer wg.Done(); instance.Run(ctx) }()
	}
	defer func() { cancel(); wg.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for d.active.Load() == nil || second.active.Load() == nil {
		if time.Now().After(deadline) {
			t.Fatal("instances did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	first, other := d.active.Load().server, second.active.Load().server
	if first.CA.PEM != other.CA.PEM || first.PublicURL != other.PublicURL {
		t.Fatal("instances loaded different control planes")
	}
	// A cookie issued by one instance authorizes the other without sticky routing.
	login, _ := json.Marshal(adminCredentials{Username: in.Username, Password: in.Password})
	r := contractRequest("POST", "https://room.test/v2/admin/login", bytes.NewReader(login))
	r.Header.Set("Origin", "https://room.test")
	w := httptest.NewRecorder()
	d.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("login: %d", w.Code)
	}
	r = contractRequest("GET", "https://room.test/v2/admin/snapshot", nil)
	r.AddCookie(w.Result().Cookies()[0])
	w = httptest.NewRecorder()
	second.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("shared session: %d", w.Code)
	}
	for _, secret := range []string{db, savedKey, in.Password, code} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("admin snapshot exposed secret")
		}
	}
}

func TestSetupConcurrentCreationAndUploadValidation(t *testing.T) {
	s, ca := database(t)
	db := setupDatabaseURL(t, s)
	d, code := setupInstance(t)
	in := createRequest(code, db)
	in.CAMode, in.CACert, in.CAKey = "upload", ca.PEM, ca.SigningPEM()
	other, err := pki.Generate(s.Network)
	must(t, err)
	in.CAKey = other.SigningPEM()
	if w := setupRequest(t, d, in, "https://room.test"); w.Code != 422 {
		t.Fatalf("mismatched CA: %d", w.Code)
	}
	var count int
	must(t, s.Pool.QueryRow(context.Background(), "SELECT count(*) FROM deployment").Scan(&count))
	if count != 0 {
		t.Fatal("invalid CA modified deployment")
	}
	in.CAKey = ca.SigningPEM()
	second, code2 := setupInstance(t)
	in2 := in
	in2.Code = code2
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for _, tc := range []struct {
		d  *Deployment
		in SetupRequest
	}{{d, in}, {second, in2}} {
		wg.Add(1)
		go func() { defer wg.Done(); statuses <- setupRequest(t, tc.d, tc.in, "https://room.test").Code }()
	}
	wg.Wait()
	close(statuses)
	got := map[int]int{}
	for status := range statuses {
		got[status]++
	}
	if got[200] != 1 || got[409] != 1 {
		t.Fatalf("concurrent create: %v", got)
	}
	api, err := LoadDeployment(context.Background(), db)
	must(t, err)
	defer api.Store.Pool.Close()
	if api.CA.PEM != ca.PEM {
		t.Fatal("uploaded CA changed")
	}
}

func TestSetupRollsBackAndRefusesOldDeployment(t *testing.T) {
	s, ca := database(t)
	in := createRequest("", setupDatabaseURL(t, s))
	encoded, err := passwordHash(in.Password)
	must(t, err)
	err = s.initialize(context.Background(), in, ca, encoded, func() error { return errors.New("disk unavailable") })
	if err == nil {
		t.Fatal("failed bind committed")
	}
	var count int
	must(t, s.Pool.QueryRow(context.Background(), "SELECT count(*) FROM administrator").Scan(&count))
	if count != 0 {
		t.Fatal("partial administrator committed")
	}
	_, err = s.Pool.Exec(context.Background(), "INSERT INTO settings(key,value) VALUES('ca_fingerprint','old')")
	must(t, err)
	err = s.initialize(context.Background(), in, ca, encoded, func() error { t.Fatal("old deployment rebound"); return nil })
	if !model.IsCode(err, "setup_already_configured") {
		t.Fatalf("old deployment accepted: %v", err)
	}
}

func TestSetupInstanceCannotBindTwoDatabases(t *testing.T) {
	first, _ := database(t)
	second, _ := database(t)
	d, code := setupInstance(t)
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, db := range []string{setupDatabaseURL(t, first), setupDatabaseURL(t, second)} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- setupRequest(t, d, createRequest(code, db), "https://room.test").Code
		}()
	}
	wg.Wait()
	close(statuses)
	got := map[int]int{}
	for status := range statuses {
		got[status]++
	}
	if got[200] != 1 || got[409] != 1 {
		t.Fatalf("database binding race: %v", got)
	}
	total := 0
	for _, s := range []*Store{first, second} {
		var count int
		must(t, s.Pool.QueryRow(context.Background(), "SELECT count(*) FROM deployment").Scan(&count))
		total += count
	}
	if total != 1 {
		t.Fatal("losing database transaction was not rolled back")
	}
}
