package control

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type adminResponse struct {
	http.ResponseWriter
	status int
}

func (w *adminResponse) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *adminResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Only authenticated failed writes need this fallback event: successful writes
// already record their concrete changes atomically. Never retain request bodies.
func (s *Server) adminWriteLogged(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := &adminResponse{ResponseWriter: w, status: 200}
		next(result, r)
		if result.status < 400 || result.status == 429 {
			return
		}
		cookie, err := r.Cookie(adminCookie)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var actor string
		if err = s.Store.Pool.QueryRow(ctx, "SELECT a.username FROM admin_sessions s CROSS JOIN administrator a WHERE s.token_hash=$1 AND s.expires_at>now()", hash(cookie.Value)).Scan(&actor); err != nil {
			return
		}
		_ = s.Store.Write(ctx, func(tx pgx.Tx) error {
			return adminEvent(ctx, tx, actor, "admin.operation_failed", r.URL.Path, map[string]any{"status": result.status, "reason": http.StatusText(result.status)})
		})
	}
}

func adminEvent(ctx context.Context, tx pgx.Tx, actor, kind, target string, detail any) error {
	b, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO admin_events(actor,kind,target,detail) VALUES($1,$2,$3,$4)", actor, kind, target, b)
	return err
}
