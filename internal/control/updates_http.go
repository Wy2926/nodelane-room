package control

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Server) registerUpdates(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/downloads", s.publicDownloads)
	mux.HandleFunc("GET /v2/downloads/{release}", s.publicDownloads)
	mux.HandleFunc("GET /v2/updates/check", s.publicUpdateCheck)
	mux.HandleFunc("POST /v2/client/report", s.playerMutation(s.playerReportClient))
	mux.HandleFunc("GET /v2/admin/updates", s.adminHandler(s.adminUpdates))
	mux.HandleFunc("PUT /v2/admin/updates/sources", s.adminWrite(s.adminMutation(s.adminUpdateSource)))
	mux.HandleFunc("POST /v2/admin/updates/sources/{source}/test", s.adminWrite(s.adminTestSource))
	mux.HandleFunc("PUT /v2/admin/updates/repository", s.adminWrite(s.adminMutationLimit(s.adminUpdateRepository, 3<<20)))
	mux.HandleFunc("PUT /v2/admin/updates/releases", s.adminWrite(s.adminMutation(s.adminUpdateRelease)))
	mux.HandleFunc("PUT /v2/admin/updates/policies", s.adminWrite(s.adminMutation(s.adminUpdatePolicy)))
	mux.HandleFunc("PUT /v2/admin/updates/releases/{release}/sources/{source}", s.adminWrite(s.adminReplica))
	mux.HandleFunc("POST /v2/admin/updates/releases/{release}/sources/{source}", s.adminWrite(s.adminReplica))
}

func (s *Server) publicDownloads(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil || s.Store.Pool == nil {
		s.fail(w, model.Failure("system_unavailable"))
		return
	}
	id := r.PathValue("release")
	if id != "" && !validResourceID(id) {
		s.fail(w, ErrNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.Store.Rate(ctx, "downloads:"+requestIP(r), 120, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	if id != "" {
		out, err := s.Store.downloadRelease(ctx, id)
		s.result(w, out, err)
		return
	}
	out, err := s.Store.downloads(ctx)
	s.result(w, out, err)
}

func (s *Server) playerReportClient(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
	var in model.ClientReport
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return reportClientUpdate(r.Context(), tx, id, in)
}

func (s *Server) adminUpdateSource(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
	var in model.UpdateSource
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return saveSource(r.Context(), tx, actor, in)
}

func (s *Server) adminUpdateRepository(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
	var in model.UpdateRepository
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return saveRepository(r.Context(), tx, actor, in)
}

func (s *Server) adminUpdateRelease(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
	var in model.UpdateRelease
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return saveRelease(r.Context(), tx, actor, in)
}

func (s *Server) adminUpdatePolicy(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
	var in model.UpdatePolicy
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return savePolicy(r.Context(), tx, actor, in)
}

func (s *Server) publicUpdateCheck(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	version, osName, arch := q.Get("version"), q.Get("os"), q.Get("arch")
	if !model.ValidVersion(version) || !model.ValidUpdatePlatform(osName, arch) {
		s.fail(w, ErrInvalid)
		return
	}
	if err := s.Store.Rate(r.Context(), "updates:"+requestIP(r), 120, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	out, err := s.Store.checkUpdate(r.Context(), version, osName, arch, q.Get("installed") == "1")
	s.result(w, out, err)
}

func (s *Server) adminUpdates(w http.ResponseWriter, r *http.Request, _ string) {
	out, err := s.Store.updateOverview(r.Context(), r.URL.Query().Get("after"))
	s.result(w, out, err)
}

func (s *Server) adminTestSource(w http.ResponseWriter, r *http.Request, actor string) {
	err := s.Store.testUpdateSource(r.Context(), actor, r.PathValue("source"))
	s.result(w, map[string]bool{"ok": true}, err)
}

func (s *Server) adminReplica(w http.ResponseWriter, r *http.Request, actor string) {
	if s.updateTransfers.Add(1) > 2 {
		s.updateTransfers.Add(-1)
		s.fail(w, ErrRateLimited)
		return
	}
	defer s.updateTransfers.Add(-1)
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(40 * time.Minute))
	_ = controller.SetWriteDeadline(time.Now().Add(40 * time.Minute))
	var upload io.Reader
	if r.Method == http.MethodPut {
		upload = r.Body
	}
	cookie, _ := r.Cookie(adminCookie)
	err := s.Store.replicateUpdate(r.Context(), actor, hash(cookie.Value), r.PathValue("source"), r.PathValue("release"), upload)
	s.result(w, map[string]bool{"ok": true}, err)
}
