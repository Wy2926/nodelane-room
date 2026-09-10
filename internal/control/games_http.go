package control

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Server) playerGames(w http.ResponseWriter, r *http.Request, _ string) {
	tx, err := s.Store.Pool.BeginTx(r.Context(), pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	out, err := readGames(r.Context(), tx, true)
	s.result(w, out, err)
}

// These are public, bounded, downloaded artwork bytes, never a URL proxy.
func (s *Server) gameImage(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var contentType string
	err := s.Store.Pool.QueryRow(r.Context(), "SELECT data,content_type FROM game_images WHERE game_id=$1 AND kind=$2", r.PathValue("game"), r.PathValue("image")).Scan(&data, &contentType)
	if err != nil {
		s.fail(w, noRows(err))
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("ETag", `"`+hash(string(data))+`"`)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}

func (s *Server) adminUpdateGame(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
	var in model.GameUpdateRequest
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return s.Store.updateGame(r.Context(), tx, actor, r.PathValue("game"), in)
}

func (s *Server) adminImportGame(w http.ResponseWriter, r *http.Request, actor string) {
	var in model.GameImportRequest
	if err := decodeRequest(w, r, &in); err != nil {
		s.fail(w, err)
		return
	}
	id, err := steamAppID(in.URL)
	if err != nil {
		s.fail(w, err)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	ctx := r.Context()
	cookie, _ := r.Cookie(adminCookie)
	check := func(tx pgx.Tx) error { return validAdminSession(ctx, tx, hash(cookie.Value)) }
	requestHash := hash("game-import:" + id)
	// Recover completed imports without depending on Steam being available.
	uncached := errors.New("import not cached")
	cached, err := s.Store.mutateChecked(ctx, "admin:"+actor, key, requestHash, check, func(pgx.Tx) (any, error) { return nil, uncached })
	if !errors.Is(err, uncached) {
		s.rawResult(w, cached, err)
		return
	}
	// Downloads precede the shared write lock. A slow website must not stall
	// room heartbeats, lease renewal or revocations on any control replica.
	if err = s.Store.Rate(r.Context(), "game-import:"+actor, 10, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	imported, err := importSteamGame(r.Context(), id, steamHTTPClient())
	if err != nil {
		s.fail(w, err)
		return
	}
	out, err := s.Store.mutateChecked(ctx, "admin:"+actor, key, requestHash, check, func(tx pgx.Tx) (any, error) {
		g := imported.Game
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM games WHERE id=$1)", g.ID).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("%w: 游戏已导入，请在列表中编辑", ErrConflict)
		}
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM games").Scan(&count); err != nil {
			return nil, err
		}
		if count >= 500 {
			return nil, fmt.Errorf("%w: 游戏列表最多 500 项", ErrConflict)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO games(id,name,summary,source_url) VALUES($1,$2,$3,$4)", g.ID, g.Name, g.Summary, g.SourceURL); err != nil {
			return nil, err
		}
		for kind, img := range imported.Images {
			if _, err := tx.Exec(ctx, "INSERT INTO game_images(game_id,kind,data,content_type) VALUES($1,$2,$3,$4)", g.ID, kind, img.Data, img.ContentType); err != nil {
				return nil, err
			}
		}
		if err := adminEvent(ctx, tx, actor, "game.imported", g.ID, map[string]string{"source": "steam"}); err != nil {
			return nil, err
		}
		return readGame(ctx, tx, g.ID)
	})
	s.rawResult(w, out, err)
}
