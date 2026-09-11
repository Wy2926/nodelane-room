package control

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Server) registerNode(mux *http.ServeMux) {
	mux.HandleFunc("POST /v2/node/telemetry", s.nodeAuth(s.nodeTelemetry))
	mux.HandleFunc("POST /v2/node/enrollment/challenge", s.nodeEnrollmentChallenge)
	mux.HandleFunc("POST /v2/node/enrollment/complete", s.nodeEnrollmentComplete)
	mux.HandleFunc("POST /v2/node/auth/challenge", s.nodeChallenge)
	mux.HandleFunc("POST /v2/node/auth/verify", s.nodeVerify)
	mux.HandleFunc("POST /v2/node/sync", s.nodeAuth(s.nodeSync))
	mux.HandleFunc("POST /v2/node/lease", s.nodeAuth(s.nodeLease))
}

func (s *Server) nodeEnrollmentChallenge(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Rate(r.Context(), "node-enroll:"+requestIP(r), 30, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	var in model.EnrollmentChallengeRequest
	if err := decodeRequest(w, r, &in); err != nil {
		s.fail(w, err)
		return
	}
	out, err := s.Store.challenge(r.Context(), in.ChallengeRequest, "enrollment", in.Key)
	s.result(w, out, err)
}

func (s *Server) nodeEnrollmentComplete(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Rate(r.Context(), "node-enroll:"+requestIP(r), 30, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	var in model.VerifyRequest
	if err := decodeRequest(w, r, &in); err != nil {
		s.fail(w, err)
		return
	}
	out, err := s.Store.verify(r.Context(), in, "enrollment")
	s.result(w, out, err)
}

func (s *Server) nodeChallenge(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Rate(r.Context(), "node-auth:"+requestIP(r), 120, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	var in model.ChallengeRequest
	if err := decodeRequest(w, r, &in); err != nil {
		s.fail(w, err)
		return
	}
	out, err := s.Store.challenge(r.Context(), in, "node", "")
	s.result(w, out, err)
}

func (s *Server) nodeVerify(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Rate(r.Context(), "node-auth:"+requestIP(r), 120, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	var in model.VerifyRequest
	if err := decodeRequest(w, r, &in); err != nil {
		s.fail(w, err)
		return
	}
	out, err := s.Store.verify(r.Context(), in, "node")
	s.result(w, out, err)
}

func (s *Server) nodeSync(w http.ResponseWriter, r *http.Request, id string) {
	var in model.NodeSyncRequest
	if err := decodeRequest(w, r, &in); err != nil {
		s.fail(w, err)
		return
	}
	out, err := s.Store.SyncNode(r.Context(), id, in)
	s.result(w, out, err)
}

func (s *Server) nodeLease(w http.ResponseWriter, r *http.Request, id string) {
	var raw json.RawMessage
	var in model.LeaseRequest
	if err := decodeRequest(w, r, &raw); err != nil {
		s.fail(w, err)
		return
	}
	if err := decodeBytes(raw, &in); err != nil {
		s.fail(w, err)
		return
	}
	out, err := s.Store.nodeLease(r.Context(), s.CA, id, r.Header.Get("Idempotency-Key"), hash(r.Method+":"+r.URL.Path+":"+string(raw)), in)
	s.rawResult(w, out, err)
}

func (s *Server) nodeAuth(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := s.Store.authenticate(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), "node")
		if err != nil {
			s.fail(w, err)
			return
		}
		if err = s.Store.Rate(r.Context(), "node:"+id, 240, time.Minute); err != nil {
			s.fail(w, err)
			return
		}
		next(w, r, id)
	}
}
