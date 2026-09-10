package control

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"
)

func (s *Server) checkOrigin(r *http.Request) bool {
	expected := strings.TrimRight(s.PublicURL, "/")
	if expected == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		expected = scheme + "://" + r.Host
	}
	return r.Header.Get("Origin") == expected && r.Header.Get("Sec-Fetch-Site") != "cross-site"
}
func (s *Server) adminAuth(w http.ResponseWriter, r *http.Request) (string, bool) {
	c, err := r.Cookie(adminCookie)
	if err != nil || len(c.Value) != 64 {
		s.fail(w, ErrUnauthorized)
		return "", false
	}
	var actor, csrf string
	err = s.Store.Pool.QueryRow(r.Context(), `SELECT a.username,s.csrf_hash FROM admin_sessions s CROSS JOIN administrator a WHERE s.token_hash=$1 AND s.expires_at>now() AND s.last_seen>now()-interval '30 minutes'`, hash(c.Value)).Scan(&actor, &csrf)
	if err != nil {
		s.fail(w, ErrUnauthorized)
		return "", false
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		if !s.checkOrigin(r) || len(r.Header.Get("X-CSRF-Token")) != 64 || subtle.ConstantTimeCompare([]byte(hash(r.Header.Get("X-CSRF-Token"))), []byte(csrf)) != 1 {
			s.fail(w, ErrForbidden)
			return "", false
		}
	}
	if _, err = s.Store.Pool.Exec(r.Context(), "UPDATE admin_sessions SET last_seen=now() WHERE token_hash=$1", hash(c.Value)); err != nil {
		s.fail(w, err)
		return "", false
	}
	return actor, true
}

func (s *Server) adminIdentity(next func(http.ResponseWriter, *http.Request, adminCredentials)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.checkOrigin(r) {
			s.fail(w, ErrForbidden)
			return
		}
		if err := s.Store.Rate(r.Context(), "admin-login:"+requestIP(r), 8, time.Minute); err != nil {
			s.fail(w, err)
			return
		}
		if err := s.Store.Rate(r.Context(), "admin-login-global", 30, time.Minute); err != nil {
			s.fail(w, err)
			return
		}
		var in adminCredentials
		if err := decodeRequest(w, r, &in); err != nil {
			s.fail(w, err)
			return
		}
		next(w, r, in)
	}
}

func (s *Server) adminBootstrap(w http.ResponseWriter, r *http.Request, in adminCredentials) {
	err := s.Store.bootstrapAdmin(r.Context(), in)
	s.result(w, map[string]bool{"ok": err == nil}, err)
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request, in adminCredentials) {
	token, err := s.Store.loginAdmin(r.Context(), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	writeJSON(w, 200, map[string]string{"username": in.Username, "csrf": hash("admin-csrf:" + token)})
}

func (s *Server) adminSession(w http.ResponseWriter, r *http.Request, actor string) {
	// Derivation keeps independent tabs usable without persisting plaintext in the database.
	cookie, _ := r.Cookie(adminCookie)
	s.result(w, map[string]string{"username": actor, "csrf": hash("admin-csrf:" + cookie.Value)}, nil)
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request, _ string) {
	cookie, _ := r.Cookie(adminCookie)
	err := s.Store.logoutAdmin(r.Context(), hash(cookie.Value))
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	s.result(w, map[string]bool{"ok": true}, err)
}

func (s *Server) adminPassword(w http.ResponseWriter, r *http.Request, actor string) {
	var in struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if err := decodeRequest(w, r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.Rate(r.Context(), "admin-password", 5, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	cookie, _ := r.Cookie(adminCookie)
	err := s.Store.changeAdminPassword(r.Context(), actor, hash(cookie.Value), in.Current, in.Password)
	s.result(w, map[string]bool{"ok": err == nil}, err)
}
