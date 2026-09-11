package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

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
	s.registerPlayer(mux)
	s.registerUsers(mux)
	s.registerOIDC(mux)
	s.registerNode(mux)
	s.registerAdmin(mux)
	s.registerAdminWeb(mux)
	s.registerInstall(mux)
	s.registerUpdates(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		defer func() {
			if v := recover(); v != nil {
				s.Log.Error("HTTP handler panic", "error", v)
				s.fail(w, errors.New("internal failure"))
			}
		}()
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
		return fmt.Errorf("%w: malformed JSON", ErrInvalid)
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return ErrInvalid
	}
	return nil
}
func decodeRequest(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return ErrInvalid
	}
	return decodeBytes(b, v)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) fail(w http.ResponseWriter, err error) {
	s.failures.Add(1)
	status := 500
	code := "internal_error"
	message := "internal server error"
	switch {
	case errors.Is(err, ErrInvalid):
		status = 400
		code = "invalid_request"
		message = err.Error()
	case errors.Is(err, ErrUnauthorized):
		status = 401
		code = "unauthorized"
		message = err.Error()
	case errors.Is(err, ErrUpdateRequired):
		status, code, message = 403, "update_required", "client update required"
	case errors.Is(err, ErrForbidden):
		status = 403
		code = "forbidden"
		message = err.Error()
	case errors.Is(err, ErrNotFound):
		status = 404
		code = "not_found"
		message = err.Error()
	case errors.Is(err, ErrConflict):
		status = 409
		code = "conflict"
		message = err.Error()
	case errors.Is(err, ErrRateLimited):
		status = 429
		code = "rate_limited"
		message = err.Error()
		w.Header().Set("Retry-After", "60")
	}
	if status == 500 {
		s.Log.Error("control request failed", "error", err)
	}
	writeJSON(w, status, map[string]string{"code": code, "message": message})
}
func (s *Server) result(w http.ResponseWriter, out any, err error) {
	if err != nil {
		s.fail(w, err)
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
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}
