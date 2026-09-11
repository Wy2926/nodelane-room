package control

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Server) registerUsers(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/me/operations/{operation}", s.playerAuth(s.operationReceipt))
	mux.HandleFunc("GET /v2/me/devices", s.playerAuth(s.playerDevices))
	mux.HandleFunc("POST /v2/me/devices/{device}/revoke", s.playerMutation(s.playerRevokeDevice))
	mux.HandleFunc("GET /v2/me", s.playerAuth(s.playerAccount))
	mux.HandleFunc("POST /v2/me/logout", s.playerMutation(s.playerLogout))
	mux.HandleFunc("POST /v2/me/takeover", s.playerMutation(s.playerTakeover))
	mux.HandleFunc("GET /v2/admin/users", s.adminHandler(s.adminUsers))
	mux.HandleFunc("GET /v2/admin/users/{user}", s.adminHandler(s.adminUser))
	mux.HandleFunc("POST /v2/admin/users/{user}/actions", s.adminWrite(s.adminMutation(s.adminUserAction)))
}

func (s *Server) playerRevokeDevice(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	var in struct{}
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return revokePlayerDevice(r.Context(), tx, id, r.PathValue("device"))
}

func (s *Server) playerLogout(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	var in struct{}
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return logoutPlayer(r.Context(), tx, id)
}

func (s *Server) playerTakeover(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	var in model.TakeoverRequest
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return takeoverPlayer(r.Context(), tx, id, in)
}

func (s *Server) playerAccount(w http.ResponseWriter, r *http.Request, id string) {
	out, err := s.Store.AccountStatus(r.Context(), id)
	s.result(w, out, err)
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request, _ string) {
	out, err := s.Store.listUsers(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), r.URL.Query().Get("after"))
	s.result(w, out, err)
}

func (s *Server) adminUser(w http.ResponseWriter, r *http.Request, _ string) {
	out, err := s.Store.userDetail(r.Context(), r.PathValue("user"))
	s.result(w, out, err)
}

func (s *Server) adminUserAction(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
	var in model.UserAction
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return userAction(r.Context(), tx, actor, r.PathValue("user"), in)
}

func (s *Server) playerDevices(w http.ResponseWriter, r *http.Request, id string) {
	out, err := s.Store.userDevices(r.Context(), id)
	s.result(w, out, err)
}
