package control

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
	"github.com/nodelane/nodelane-room/internal/platform"
)

type setupCode struct {
	Hash    string    `json:"hash"`
	Expires time.Time `json:"expires"`
}

func DatabaseURL(dir string) (string, error) {
	b, err := platform.LoadPrivateFile(filepath.Join(dir, "database.bin"))
	return string(b), err
}

// BootstrapCode is a local-console capability. Only its hash is persisted and it
// never enters service logs. Each independently deployed instance has its own directory.
func BootstrapCode(ctx context.Context, dir string) (string, error) {
	if databaseURL, err := DatabaseURL(dir); err == nil {
		s, err := Open(ctx, databaseURL, model.DefaultPool)
		if err != nil {
			return "", errors.New("cannot verify initialization while database is unavailable")
		}
		defer s.Pool.Close()
		var exists bool
		if err = s.Pool.QueryRow(ctx, "SELECT to_regclass('administrator') IS NOT NULL").Scan(&exists); err != nil {
			return "", errors.New("cannot verify database initialization")
		}
		if exists {
			if err = s.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM administrator)").Scan(&exists); err != nil || exists {
				return "", model.Failure("setup_already_configured")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", model.Failure("setup_binding_failed")
	}
	token := randomID() + randomID()
	b, err := json.Marshal(setupCode{Hash: hash(token), Expires: time.Now().Add(10 * time.Minute)})
	if err != nil {
		return "", err
	}
	if err = platform.SavePrivateFile(filepath.Join(dir, "bootstrap.bin"), b, true); err != nil {
		return "", err
	}
	return token, nil
}

type runningControl struct {
	server  *Server
	handler http.Handler
}

// Deployment exposes the public site and private setup entry before PostgreSQL has been configured, then
// atomically publishes an immutable API server loaded from shared database state.
type Deployment struct {
	GeoIP       *GeoIP
	AdminPath   string
	StateDir    string
	ReleaseDir  string
	BehindProxy bool
	Log         *slog.Logger
	active      atomic.Pointer[runningControl]
}

func (d *Deployment) validCode(code string) bool {
	if len(code) != 64 {
		return false
	}
	b, err := platform.LoadPrivateFile(filepath.Join(d.StateDir, "bootstrap.bin"))
	var grant setupCode
	return err == nil && json.Unmarshal(b, &grant) == nil && time.Now().Before(grant.Expires) && subtle.ConstantTimeCompare([]byte(hash(code)), []byte(grant.Hash)) == 1
}

func (d *Deployment) setup(w http.ResponseWriter, r *http.Request) {
	reply := &Server{Log: d.Log}
	if d.active.Load() != nil {
		reply.fail(w, model.Failure("setup_already_configured"))
		return
	}
	// Before PUBLIC_URL exists, accept only the browser's same origin. Forwarded
	// headers cannot select a trusted origin; TLS termination is explicitly enabled.
	scheme := "http"
	if r.TLS != nil || d.BehindProxy {
		scheme = "https"
	}
	origin := scheme + "://" + r.Host
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || r.Header.Get("Origin") != origin || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		reply.fail(w, model.Failure("request_origin_rejected"))
		return
	}
	var in SetupRequest
	if err = decodeRequest(w, r, &in); err != nil {
		reply.fail(w, err)
		return
	}
	if !d.validCode(in.Code) {
		reply.fail(w, model.Failure("setup_code_unusable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	if err = in.normalize(); err != nil {
		reply.fail(w, err)
		return
	}
	bind := func() error { return d.bind(in) }
	if in.Mode == "connect" {
		if _, e := DatabaseURL(d.StateDir); !errors.Is(e, os.ErrNotExist) {
			reply.fail(w, model.Failure("setup_already_configured"))
			return
		}
		api, e := LoadDeployment(ctx, in.DatabaseURL)
		if e != nil {
			reply.fail(w, e)
			return
		}
		defer api.Store.Pool.Close()
		e = api.Store.Rate(ctx, "control-connect", 8, time.Minute)
		if e == nil {
			e = api.Store.connectDeployment(ctx, in, bind)
		}
		reply.result(w, map[string]any{"ok": e == nil, "public_url": api.PublicURL}, e)
		return
	}
	if in.PublicURL != origin {
		reply.fail(w, model.Failure("request_origin_rejected"))
		return
	}
	n, err := parseNetwork(in.Network)
	if err != nil {
		reply.fail(w, err)
		return
	}
	var ca *pki.Authority
	if in.CAMode == "generate" {
		ca, err = pki.Generate(n)
	} else {
		ca, err = pki.Parse([]byte(in.CACert), []byte(in.CAKey))
	}
	if err != nil || ca.ValidatePool(n) != nil {
		reply.fail(w, model.Failure("setup_ca_invalid"))
		return
	}
	if bound, e := DatabaseURL(d.StateDir); e == nil && bound != in.DatabaseURL {
		reply.fail(w, model.Failure("setup_already_configured"))
		return
	} else if e != nil && !errors.Is(e, os.ErrNotExist) {
		reply.fail(w, model.Failure("setup_binding_failed"))
		return
	}
	s, err := Open(ctx, in.DatabaseURL, in.Network)
	if err != nil {
		reply.fail(w, model.Failure("system_unavailable"))
		return
	}
	defer s.Pool.Close()
	encoded, err := passwordHash(in.Password)
	if err == nil {
		err = s.initialize(ctx, in, ca, encoded, bind)
	}
	reply.result(w, map[string]any{"ok": err == nil, "public_url": in.PublicURL}, err)
}

func (d *Deployment) bind(in SetupRequest) error {
	if !d.validCode(in.Code) {
		return model.Failure("setup_code_unusable")
	}
	path := filepath.Join(d.StateDir, "database.bin")
	if err := platform.SavePrivateFile(path, []byte(in.DatabaseURL), false); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return model.Failure("setup_binding_failed")
		}
		bound, err := DatabaseURL(d.StateDir)
		if err != nil || bound != in.DatabaseURL {
			return model.Failure("setup_already_configured")
		}
	}
	return nil
}

func (d *Deployment) Handler() http.Handler {
	mux := http.NewServeMux()
	web := &Server{Log: d.Log, AdminPath: d.AdminPath}
	web.registerAdminWeb(mux)
	registerSiteWeb(mux)
	mux.HandleFunc("GET /healthz", web.health)
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 503, map[string]string{"status": "setup_required"})
	})
	mux.HandleFunc("GET /v2/capabilities", web.capabilities)
	mux.HandleFunc("POST /v2/admin/setup", d.setup)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v2/") {
			http.NotFound(w, r)
			return
		}
		web.fail(w, model.Failure("system_unavailable"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		active := d.active.Load()
		if strings.HasPrefix(r.URL.Path, "/v2/") && (active == nil || r.URL.Path == "/v2/admin/setup") {
			cw := &contractWriter{ResponseWriter: w, requestID: randomID()}
			w = cw
			w.Header().Set("X-Request-ID", cw.requestID)
			if r.URL.Path != "/v2/capabilities" && r.Header.Get(model.ContractHeader) != model.Contract {
				web.fail(w, model.Failure("api_contract_unsupported"))
				return
			}
		}
		if r.URL.Path == "/v2/admin/setup" && (r.Method == "GET" || r.Method == "HEAD") {
			_, err := os.Stat(filepath.Join(d.StateDir, "database.bin"))
			writeJSON(w, 200, map[string]bool{"initialized": active != nil, "configured": !errors.Is(err, os.ErrNotExist)})
			return
		}
		if active != nil {
			if r.URL.Path == "/v2/admin/setup" {
				web.fail(w, model.Failure("setup_already_configured"))
				return
			}
			active.handler.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// Run restores this instance from its private locator and shared database state.
func (d *Deployment) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	defer func() {
		if active := d.active.Load(); active != nil {
			active.server.Store.Pool.Close()
		}
	}()
	lastSweep := time.Time{}
	lastError := ""
	for {
		active := d.active.Load()
		if active == nil {
			if db, err := DatabaseURL(d.StateDir); err == nil {
				loadCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				api, err := LoadDeployment(loadCtx, db)
				cancel()
				if err == nil {
					api.Log, api.ReleaseDir = d.Log, d.ReleaseDir
					api.GeoIP = d.GeoIP
					api.AdminPath = d.AdminPath
					d.active.Store(&runningControl{server: api, handler: api.Handler()})
					d.Log.Info("control configuration loaded")
				} else if err.Error() != lastError && ctx.Err() == nil {
					lastError = err.Error()
					d.Log.Error("control configuration unavailable", "code", model.Code(err))
				}
			} else if !errors.Is(err, os.ErrNotExist) && lastError != "locator" {
				lastError = "locator"
				d.Log.Error("cannot read private database locator")
			}
		} else if time.Since(lastSweep) >= 10*time.Second {
			active.server.telemetry.prune(time.Now())
			if err := active.server.Store.Sweep(ctx); err != nil && ctx.Err() == nil {
				d.Log.Error("maintenance failed")
			}
			lastSweep = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
