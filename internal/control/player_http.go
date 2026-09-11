package control

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Server) registerPlayer(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/rooms", s.playerAuth(s.playerOwnedRooms))
	mux.HandleFunc("GET /v2/rooms/{room}/manage", s.playerAuth(s.playerRoomManagement))
	mux.HandleFunc("GET /v2/games", s.playerAuth(s.playerGames))
	mux.HandleFunc("GET /v2/games/{game}/images/{image}", s.gameImage)
	mux.HandleFunc("POST /v2/rooms/{room}/telemetry", s.playerAuth(s.playerTelemetry))
	mux.HandleFunc("POST /v2/auth/challenge", s.playerChallenge)
	mux.HandleFunc("POST /v2/auth/guest/challenge", s.playerChallenge)
	mux.HandleFunc("POST /v2/auth/guest/verify", s.playerVerify)
	mux.HandleFunc("POST /v2/auth/verify", s.playerVerify)
	mux.HandleFunc("GET /v2/rooms/{room}", s.playerAuth(s.playerSnapshot))
	mux.HandleFunc("GET /v2/rooms/{room}/events", s.playerAuth(s.playerEvents))
	mux.HandleFunc("POST /v2/rooms", s.playerMutation(s.playerCreateRoom))
	mux.HandleFunc("POST /v2/rooms/join", s.playerMutation(s.playerJoinRoom))
	mux.HandleFunc("POST /v2/rooms/{room}/lease", s.playerMutation(s.playerLease))
	mux.HandleFunc("POST /v2/rooms/{room}/heartbeat", s.playerMutation(s.playerHeartbeat))
	mux.HandleFunc("POST /v2/rooms/{room}/invite", s.playerMutation(s.playerInvite))
	mux.HandleFunc("POST /v2/rooms/{room}/kick", s.playerMutation(s.playerKick))
	mux.HandleFunc("POST /v2/rooms/{room}/transfer", s.playerMutation(s.playerTransfer))
	mux.HandleFunc("POST /v2/rooms/{room}/leave", s.playerMutation(s.playerLeave))
	mux.HandleFunc("POST /v2/rooms/{room}/close", s.playerMutation(s.playerClose))
}

func (s *Server) playerChallenge(w http.ResponseWriter, r *http.Request) {
	var in model.ChallengeRequest
	if err := decodeRequest(w, r, &in); err != nil {
		s.fail(w, err)
		return
	}
	ip := requestIP(r)
	if err := s.Store.Rate(r.Context(), "auth:"+ip, 120, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	scope := "player"
	if r.URL.Path == "/v2/auth/guest/challenge" {
		scope = "guest"
		if err := s.Store.Rate(r.Context(), "guest:"+ip, 100, time.Hour); err != nil {
			s.fail(w, err)
			return
		}
	}
	out, err := s.Store.challenge(r.Context(), in, scope, "")
	s.result(w, out, err)
}

func (s *Server) playerVerify(w http.ResponseWriter, r *http.Request) {
	var in model.VerifyRequest
	if err := decodeRequest(w, r, &in); err != nil {
		s.fail(w, err)
		return
	}
	scope := "player"
	if r.URL.Path == "/v2/auth/guest/verify" {
		scope = "guest"
	}
	out, err := s.Store.verify(r.Context(), in, scope)
	s.result(w, out, err)
}

func (s *Server) playerAuth(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := s.Store.Authenticate(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if err != nil {
			s.fail(w, err)
			return
		}
		if err = s.Store.Rate(r.Context(), "device:"+id, 240, time.Minute); err != nil {
			s.fail(w, err)
			return
		}
		next(w, r, id)
	}
}

func (s *Server) playerMutation(next func(*http.Request, pgx.Tx, string, []byte) (any, error)) http.HandlerFunc {
	return s.playerAuth(func(w http.ResponseWriter, r *http.Request, id string) {
		r.Body = http.MaxBytesReader(w, r.Body, 65536)
		b, err := io.ReadAll(r.Body)
		if err != nil {
			s.fail(w, ErrInvalid)
			return
		}
		check := func(tx pgx.Tx) error {
			if err := validPlayerSession(r.Context(), tx, id, hash(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))); err != nil {
				return err
			}
			if r.URL.Path == "/v2/rooms" || r.URL.Path == "/v2/rooms/join" || strings.HasSuffix(r.URL.Path, "/lease") || strings.HasSuffix(r.URL.Path, "/heartbeat") {
				if err := requireUpdated(r.Context(), tx, id); err != nil {
					return err
				}
			}
			if strings.HasSuffix(r.URL.Path, "/lease") {
				if err := activeMember(r.Context(), tx, r.PathValue("room"), id); err != nil {
					return err
				}
				return validCachedLease(r.Context(), tx, id, r.Header.Get("Idempotency-Key"))
			}
			return nil
		}
		out, err := s.Store.mutateChecked(r.Context(), id, r.Header.Get("Idempotency-Key"), hash(r.Method+":"+r.URL.Path+":"+string(b)), check, func(tx pgx.Tx) (any, error) {
			return next(r, tx, id, b)
		})
		s.rawResult(w, out, err)
	})
}
