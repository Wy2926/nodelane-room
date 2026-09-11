package control

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (s *Server) registerAdmin(mux *http.ServeMux) {
	mux.HandleFunc("POST /v2/admin/games/import", s.adminWrite(s.adminImportGame))
	mux.HandleFunc("PUT /v2/admin/games/{game}", s.adminWrite(s.adminMutation(s.adminUpdateGame)))
	mux.HandleFunc("GET /v2/admin/telemetry", s.adminHandler(s.adminTelemetry))
	mux.HandleFunc("POST /v2/admin/login", s.adminIdentity(s.adminLogin))
	mux.HandleFunc("GET /v2/admin/session", s.adminHandler(s.adminSession))
	mux.HandleFunc("POST /v2/admin/logout", s.adminWrite(s.adminLogout))
	mux.HandleFunc("POST /v2/admin/password", s.adminWrite(s.adminPassword))
	mux.HandleFunc("GET /v2/admin/snapshot", s.adminHandler(s.adminOverview))
	mux.HandleFunc("GET /v2/admin/events", s.adminHandler(s.adminEvents))
	mux.HandleFunc("POST /v2/admin/nodes", s.adminWrite(s.adminMutation(s.adminCreateNode)))
	mux.HandleFunc("PUT /v2/admin/nodes/{node}", s.adminWrite(s.adminMutation(s.adminUpdateNode)))
	mux.HandleFunc("POST /v2/admin/nodes/{node}/key", s.adminWrite(s.adminNodeKey))
	mux.HandleFunc("POST /v2/admin/nodes/{node}/actions", s.adminWrite(s.adminMutation(s.adminNodeAction)))
	mux.HandleFunc("GET /v2/admin/nodes/{node}/compose", s.adminHandler(s.adminNodeCompose))
	mux.HandleFunc("GET /v2/admin/rooms/{room}", s.adminHandler(s.adminRoom))
	mux.HandleFunc("POST /v2/admin/rooms/{room}/actions", s.adminWrite(s.adminMutation(s.adminRoomAction)))
}

func (s *Server) adminHandler(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := s.adminAuth(w, r)
		if ok {
			next(w, r, actor)
		}
	}
}

func (s *Server) adminWrite(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return s.adminWriteLogged(s.adminHandler(next))
}

func (s *Server) adminMutation(next func(*http.Request, pgx.Tx, string, []byte) (any, error)) func(http.ResponseWriter, *http.Request, string) {
	return func(w http.ResponseWriter, r *http.Request, actor string) {
		limit := int64(65536)
		if r.URL.Path == "/v2/admin/updates/repository" {
			limit = 3 << 20
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			s.fail(w, ErrInvalid)
			return
		}
		ctx := r.Context()
		cookie, _ := r.Cookie(adminCookie)
		check := func(tx pgx.Tx) error { return validAdminSession(ctx, tx, hash(cookie.Value)) }
		out, err := s.Store.mutateChecked(ctx, "admin:"+actor, r.Header.Get("Idempotency-Key"), hash(r.URL.Path+":"+r.Method+":"+string(raw)), check, func(tx pgx.Tx) (any, error) {
			return next(r, tx, actor, raw)
		})
		s.rawResult(w, out, err)
	}
}
