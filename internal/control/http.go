package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
)

type Server struct {
	telemetry       telemetryMemory
	GeoIP           *GeoIP
	AdminPath       string
	Store           *Store
	CA              *pki.Authority
	Log             *slog.Logger
	PublicURL       string
	Registry        string
	ReleaseDir      string
	requests        atomic.Uint64
	failures        atomic.Uint64
	streams         atomic.Int64
	updateTransfers atomic.Int64
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("GET /metrics", s.metrics)
	mux.HandleFunc("GET /v2/capabilities", s.capabilities)
	s.registerPlayer(mux)
	s.registerUsers(mux)
	s.registerOIDC(mux)
	s.registerNode(mux)
	s.registerAdmin(mux)
	s.registerAdminWeb(mux)
	registerSiteWeb(mux)
	s.registerInstall(mux)
	s.registerUpdates(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/") {
			cw := &contractWriter{ResponseWriter: w, requestID: randomID(), operationID: r.Header.Get("Idempotency-Key")}
			if s.Store != nil && s.Store.Pool != nil {
				ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
				var at time.Time
				if s.Store.Pool.QueryRow(ctx, "SELECT now()").Scan(&at) == nil {
					cw.serverTime = &at
				}
				cancel()
			}
			w = cw
			r = requestOperation(r, cw.requestID)
			w.Header().Set("X-Request-ID", cw.requestID)
			public := r.URL.Path == "/v2/capabilities" || r.URL.Path == "/v2/updates/check" || strings.Contains(r.URL.Path, "/images/") || strings.HasPrefix(r.URL.Path, "/v2/auth/oidc/browser") || strings.HasPrefix(r.URL.Path, "/v2/auth/oidc/callback") || r.URL.Path == "/v2/auth/oidc/confirm" || (r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/compose"))
			if !public && r.Header.Get(model.ContractHeader) != model.Contract {
				s.fail(w, model.Failure("api_contract_unsupported"))
				return
			}
		}
		s.requests.Add(1)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		defer func() {
			if v := recover(); v != nil {
				if s.Log != nil {
					s.Log.Error("HTTP handler panic")
				}
				s.fail(w, errors.New("internal failure"))
			}
		}()

		if strings.HasPrefix(r.URL.Path, "/v2/") {
			_, pattern := mux.Handler(r)
			if pattern == "" {
				methods := []string{}
				for _, method := range []string{"GET", "HEAD", "POST", "PUT", "DELETE", "PATCH"} {
					probe := r.Clone(r.Context())
					probe.Method = method
					if _, p := mux.Handler(probe); p != "" {
						methods = append(methods, method)
					}
				}
				if len(methods) > 0 {
					w.Header().Set("Allow", strings.Join(methods, ", "))
					s.fail(w, model.Failure("request_method_unsupported"))
				} else {
					s.fail(w, model.Failure("resource_not_found"))
				}
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.Pool.Ping(ctx); err != nil || time.Until(s.CA.Certificate.NotAfter()) < model.LeaseDuration {
		writeJSON(w, 503, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "nodelane_http_requests_total %d\nnodelane_http_errors_total %d\nnodelane_sse_connections %d\nnodelane_ca_expiry_timestamp_seconds %d\n", s.requests.Load(), s.failures.Load(), s.streams.Load(), s.CA.Certificate.NotAfter().Unix())
}

func decodeBytes(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return model.Failure("request_malformed")
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return model.Failure("request_malformed")
	}
	return nil
}
func decodeRequest(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Header.Get("Content-Type") != "" && !jsonContentType(r) {
		return model.Failure("request_media_unsupported")
	}
	return decodeLimitedRequest(w, r, v, 65536)
}

func jsonContentType(r *http.Request) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && media == "application/json"
}

func writeSSE(w http.ResponseWriter, kind, id string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if id != "" {
		id = "id: " + id + "\n"
	}
	if _, err = fmt.Fprintf(w, "%sevent: %s\ndata: %s\n\n", id, kind, b); err != nil {
		return err
	}
	return controller.Flush()
}

func decodeLimitedRequest(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return model.Failure("request_too_large")
	}
	return decodeBytes(b, v)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	if cw := responseContract(w); cw != nil {
		var result model.Result
		switch value := v.(type) {
		case model.Result:
			result = value
		default:
			result = model.NewResult("ok", "control", cw.requestID, v)
		}
		result.RequestID = cw.requestID
		if cw.operationID != "" {
			result.OperationID = cw.operationID
		}
		if result.ServerTime == nil {
			result.ServerTime = cw.serverTime
		}
		result.Details = model.SafeDetails(result.Code, result.Details)
		v = result
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) fail(w http.ResponseWriter, err error) {
	s.failures.Add(1)
	code := model.Code(err)
	var business *model.BusinessError
	var database *pgconn.PgError
	var network net.Error
	switch {
	case errors.As(err, &business):
		code = business.Code
	case errors.Is(err, ErrInvalid):
		code = "request_validation_failed"
	case errors.Is(err, ErrUnauthorized):
		code = "auth_session_required"
	case errors.Is(err, ErrUpdateRequired):
		code = "client_update_required"
	case errors.Is(err, ErrForbidden):
		code = "resource_not_found"
	case errors.Is(err, ErrNotFound):
		code = "resource_not_found"
	case errors.Is(err, ErrConflict):
		code = "request_state_stale"
	case errors.Is(err, ErrRateLimited):
		code = "request_rate_limited"
		w.Header().Set("Retry-After", "60")
	case errors.As(err, &network), errors.Is(err, context.DeadlineExceeded):
		code = "system_unavailable"
	case errors.As(err, &database):
		if strings.HasPrefix(database.Code, "08") || strings.HasPrefix(database.Code, "53") || database.Code == "57P01" {
			code = "system_unavailable"
		}
	}
	out := model.NewResult(code, "control", randomID(), nil)
	if business != nil {
		out.Details = business.Details
	}
	if code == "request_rate_limited" {
		out.Retry.AfterMS = 60000
		w.Header().Set("Retry-After", "60")
	}
	if model.HTTPStatus(code) >= 500 && s.Log != nil {
		s.Log.Error("control request failed", "code", code, "request_id", w.Header().Get("X-Request-ID"))
	}
	writeJSON(w, model.HTTPStatus(code), out)
}
func (s *Server) result(w http.ResponseWriter, out any, err error) {
	if err != nil {
		s.fail(w, err)
		return
	}
	if result, ok := out.(model.Result); ok {
		writeJSON(w, model.HTTPStatus(result.Code), result)
		return
	}
	writeJSON(w, 200, out)
}

func requestIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func (s *Server) rawResult(w http.ResponseWriter, out []byte, err error) {
	if err != nil {
		s.fail(w, err)
		return
	}
	var receipt model.Receipt
	if err := json.Unmarshal(out, &receipt); err != nil || receipt.Result.Contract != model.Contract {
		s.fail(w, model.Failure("system_internal_error"))
		return
	}
	writeJSON(w, receipt.Status, receipt.Result)
}

type contractWriter struct {
	http.ResponseWriter
	requestID, operationID string
	serverTime             *time.Time
}

func (w *contractWriter) FlushError() error {
	return http.NewResponseController(w.ResponseWriter).Flush()
}
func (w *contractWriter) Flush()                      { _ = w.FlushError() }
func (w *contractWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func responseContract(w http.ResponseWriter) *contractWriter {
	for {
		if cw, ok := w.(*contractWriter); ok {
			return cw
		}
		unwrap, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return nil
		}
		w = unwrap.Unwrap()
	}
}

func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	enabled := false
	ready := s.Store != nil && s.Store.Pool != nil
	if ready {
		cfg, _, err := s.oidcConfig(r.Context())
		enabled = err == nil && cfg.Enabled
		ready = s.Store.Pool.Ping(r.Context()) == nil
	}
	s.result(w, map[string]any{"contract": model.Contract, "lan_version": model.LANVersion, "local_protocol_version": 3, "oidc_enabled": enabled, "ready": ready}, nil)
}
