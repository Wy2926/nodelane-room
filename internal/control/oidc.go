package control

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/hex"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
	"golang.org/x/oauth2"
)

func (s *Server) registerOIDC(mux *http.ServeMux) {
	mux.HandleFunc("POST /v2/auth/oidc/start", func(w http.ResponseWriter, r *http.Request) { s.oidcStart(w, r, "") })
	mux.HandleFunc("POST /v2/me/identity", s.playerAuth(s.oidcStart))
	mux.HandleFunc("POST /v2/auth/oidc/claim", s.oidcClaim)
	mux.HandleFunc("GET /v2/auth/oidc/browser", s.oidcBrowser)
	mux.HandleFunc("GET /v2/auth/oidc/callback", s.oidcCallback)
	mux.HandleFunc("POST /v2/auth/oidc/confirm", s.oidcConfirm)
	mux.HandleFunc("GET /v2/admin/oidc", s.adminHandler(func(w http.ResponseWriter, r *http.Request, _ string) {
		var out model.OIDCSettings
		err := s.Store.Pool.QueryRow(r.Context(), `SELECT issuer,client_id,enabled FROM oidc_settings WHERE id=1`).Scan(&out.Issuer, &out.ClientID, &out.Enabled)
		if err == pgx.ErrNoRows {
			err = nil
		}
		s.result(w, out, err)
	}))
	mux.HandleFunc("PUT /v2/admin/oidc", s.adminWrite(s.adminMutation(s.adminOIDC)))
}

func validIssuer(issuer string) bool {
	u, e := url.Parse(issuer)
	return e == nil && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")))
}

func (s *Server) adminOIDC(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
	var in model.OIDCSettings
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	if !validIssuer(in.Issuer) || len(in.Issuer) > 2048 || !model.ValidLabel(in.ClientID, 256) || len(in.ClientSecret) > 4096 {
		return nil, ErrInvalid
	}
	// Retain an omitted secret only for the same issuer and client.
	_, err := tx.Exec(r.Context(), `INSERT INTO oidc_settings(id,issuer,client_id,client_secret,enabled) VALUES(1,$1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET issuer=$1,client_id=$2,client_secret=CASE WHEN $3='' AND oidc_settings.issuer=$1 AND oidc_settings.client_id=$2 THEN oidc_settings.client_secret ELSE $3 END,enabled=$4,revision=oidc_settings.revision+1`, in.Issuer, in.ClientID, in.ClientSecret, in.Enabled)
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM login_transactions`)
	}
	if err == nil {
		err = adminEvent(r.Context(), tx, actor, "oidc.configured", "oidc", map[string]any{"issuer": in.Issuer, "client_id": in.ClientID, "enabled": in.Enabled})
	}
	return map[string]bool{"ok": err == nil}, err
}

func (s *Server) oidcConfig(ctx context.Context) (model.OIDCSettings, int64, error) {
	var c model.OIDCSettings
	var rev int64
	err := s.Store.Pool.QueryRow(ctx, `SELECT issuer,client_id,client_secret,enabled,revision FROM oidc_settings WHERE id=1 AND enabled`).Scan(&c.Issuer, &c.ClientID, &c.ClientSecret, &c.Enabled, &rev)
	if err == pgx.ErrNoRows {
		err = ErrNotFound
	}
	return c, rev, err
}

func (s *Server) oidcClient(ctx context.Context, c model.OIDCSettings) (*oidc.Provider, oauth2.Config, error) {
	provider, err := oidc.NewProvider(ctx, c.Issuer)
	if err != nil {
		return nil, oauth2.Config{}, ErrForbidden
	}
	return provider, oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: strings.TrimRight(s.PublicURL, "/") + "/v2/auth/oidc/callback", Scopes: []string{oidc.ScopeOpenID, "profile"}}, nil
}

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request, device string) {
	var in model.LoginStart
	if err := decodeRequest(w, r, &in); err != nil {
		s.fail(w, err)
		return
	}
	proof, e := hex.DecodeString(in.Proof)
	if e != nil || len(proof) != 32 || len(in.PublicKey) != ed25519.PublicKeySize || deviceID(in.PublicKey) != in.DeviceID || !model.ValidLabel(in.Name, 80) || !ed25519.Verify(in.PublicKey, []byte("nodelane-login-start:"+in.DeviceID+":"+in.Proof), in.Signature) {
		s.fail(w, ErrInvalid)
		return
	}
	if device != "" && device != in.DeviceID {
		s.fail(w, ErrForbidden)
		return
	}
	if err := s.Store.Rate(r.Context(), "oidc-start:"+requestIP(r), 30, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	_, rev, err := s.oidcConfig(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	if s.PublicURL == "" {
		s.fail(w, ErrInvalid)
		return
	}
	out := model.LoginAttempt{ID: randomID(), ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}
	out.URL = s.PublicURL + "/v2/auth/oidc/browser?id=" + out.ID
	err = s.Store.Write(r.Context(), func(tx pgx.Tx) error {
		var user, session *string
		if device != "" {
			h := hash(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if e := validPlayerSession(r.Context(), tx, device, h); e != nil {
				return e
			}
			u, e := playerUser(r.Context(), tx, device)
			if e != nil {
				return e
			}
			if u.Kind != "guest" {
				return ErrConflict
			}
			user = &u.ID
			session = &h
		} else {
			var exists bool
			if e := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM devices WHERE id=$1)`, in.DeviceID).Scan(&exists); e != nil {
				return e
			}
			if exists {
				return ErrConflict
			}
		}
		_, e := tx.Exec(r.Context(), `INSERT INTO login_transactions(id,device_id,public_key,name,proof_hash,link_user_id,session_hash,config_revision,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, out.ID, in.DeviceID, in.PublicKey, in.Name, in.Proof, user, session, rev, out.ExpiresAt)
		return e
	})
	s.result(w, out, err)
}

func loginCookie(id string) string { return "nlroom-login-" + id }

func (s *Server) oidcBrowser(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if len(id) != 32 {
		s.fail(w, ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	c, rev, err := s.oidcConfig(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	_, config, err := s.oidcClient(ctx, c)
	if err != nil {
		s.fail(w, err)
		return
	}
	browser, state, nonce, verifier := randomID()+randomID(), randomID()+randomID(), randomID()+randomID(), oauth2.GenerateVerifier()
	tag, err := s.Store.Pool.Exec(ctx, `UPDATE login_transactions SET browser_hash=$2,state_hash=$3,nonce=$4,verifier=$5,phase='authorizing' WHERE id=$1 AND phase='pending' AND expires_at>now() AND config_revision=$6`, id, hash(browser), hash(state), nonce, verifier, rev)
	if err != nil {
		s.fail(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		s.fail(w, ErrConflict)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: loginCookie(id), Value: browser, Path: "/v2/auth/oidc/", HttpOnly: true, Secure: strings.HasPrefix(s.PublicURL, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: 300})
	http.Redirect(w, r, config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce)), http.StatusSeeOther)
}

var loginPage = template.Must(template.New("login").Parse(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>NodeLane</title><main><h1>NodeLane</h1><p>{{.Message}}</p>{{if .ID}}<p>设备：{{.Name}} · {{.Device}}</p><p>仅确认你刚刚在客户端发起的请求。确认后，此设备可以使用该账号。</p><form method="post" action="/v2/auth/oidc/confirm"><input type="hidden" name="id" value="{{.ID}}"><input type="hidden" name="csrf" value="{{.CSRF}}"><button type="submit">确认登录此设备</button></form>{{end}}<p>完成后返回 NodeLane 客户端。</p></main></html>`))

func renderLogin(w http.ResponseWriter, message, id, name, device, csrf string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Keep the form POST's Origin without leaking callback code/state in Referer.
	// no-referrer makes browsers send Origin: null, which checkOrigin rejects.
	w.Header().Set("Referrer-Policy", "strict-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	_ = loginPage.Execute(w, struct{ Message, ID, Name, Device, CSRF string }{message, id, name, device, csrf})
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	state := r.URL.Query().Get("state")
	if len(state) != 64 {
		s.fail(w, ErrInvalid)
		return
	}
	var id, browser, nonce, verifier, name, device string
	var revision int64
	err := s.Store.Pool.QueryRow(ctx, `SELECT id,browser_hash,nonce,verifier,name,device_id,config_revision FROM login_transactions WHERE state_hash=$1 AND phase='authorizing' AND expires_at>now()`, hash(state)).Scan(&id, &browser, &nonce, &verifier, &name, &device, &revision)
	if err != nil {
		s.fail(w, ErrForbidden)
		return
	}
	cookie, err := r.Cookie(loginCookie(id))
	if err != nil || subtle.ConstantTimeCompare([]byte(hash(cookie.Value)), []byte(browser)) != 1 {
		s.fail(w, ErrForbidden)
		return
	}
	// Claim the callback before exchanging its one-use authorization code.
	tag, err := s.Store.Pool.Exec(ctx, `UPDATE login_transactions SET phase='exchanging',verifier=NULL WHERE id=$1 AND phase='authorizing'`, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		s.fail(w, ErrConflict)
		return
	}
	success := false
	defer func() {
		if !success {
			_, _ = s.Store.Pool.Exec(context.WithoutCancel(ctx), `UPDATE login_transactions SET phase='failed' WHERE id=$1 AND phase='exchanging'`, id)
		}
	}()
	if r.URL.Query().Get("error") != "" {
		renderLogin(w, "登录未完成，请返回客户端重试。", "", "", "", "")
		return
	}
	c, rev, err := s.oidcConfig(ctx)
	if err != nil || rev != revision {
		s.fail(w, ErrForbidden)
		return
	}
	provider, config, err := s.oidcClient(ctx, c)
	if err != nil {
		s.fail(w, err)
		return
	}
	token, err := config.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
	if err != nil {
		s.fail(w, ErrForbidden)
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		s.fail(w, ErrForbidden)
		return
	}
	verified, err := provider.Verifier(&oidc.Config{ClientID: c.ClientID}).Verify(ctx, raw)
	if err != nil || verified.Nonce != nonce || verified.Subject == "" || len(verified.Subject) > 255 {
		s.fail(w, ErrForbidden)
		return
	}
	var claims struct {
		AZP string `json:"azp"`
	}
	if verified.Claims(&claims) != nil || (claims.AZP != "" && claims.AZP != c.ClientID) || (len(verified.Audience) > 1 && claims.AZP == "") {
		s.fail(w, ErrForbidden)
		return
	}
	tag, err = s.Store.Pool.Exec(ctx, `UPDATE login_transactions SET phase='verified',issuer=$2,subject=$3,nonce=NULL WHERE id=$1 AND phase='exchanging' AND expires_at>now() AND config_revision=(SELECT revision FROM oidc_settings WHERE id=1 AND enabled)`, id, verified.Issuer, verified.Subject)
	if err != nil {
		s.fail(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		s.fail(w, ErrConflict)
		return
	}
	success = true
	renderLogin(w, "身份验证成功，请确认授权设备。", id, name, device[:12], hash("confirm:"+cookie.Value))
}

// Linking preserves the guest's user ID. Existing identities never merge users.
func completeIdentity(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	var device, name, issuer, subject string
	var pub []byte
	var link, session *string
	err := tx.QueryRow(ctx, `SELECT device_id,name,public_key,issuer,subject,link_user_id,session_hash FROM login_transactions WHERE id=$1 AND phase='verified' AND expires_at>now() AND config_revision=(SELECT revision FROM oidc_settings WHERE id=1 AND enabled)`, id).Scan(&device, &name, &pub, &issuer, &subject, &link, &session)
	if err != nil {
		return "", ErrForbidden
	}
	var user string
	err = tx.QueryRow(ctx, `SELECT user_id FROM user_identities WHERE issuer=$1 AND subject=$2`, issuer, subject).Scan(&user)
	if err != nil && err != pgx.ErrNoRows {
		return "", err
	}
	if link != nil {
		if session == nil {
			return "", ErrForbidden
		}
		if err = validPlayerSession(ctx, tx, device, *session); err != nil {
			return "", err
		}
		u, e := playerUser(ctx, tx, device)
		if e != nil {
			return "", e
		}
		if u.ID != *link || u.Kind != "guest" {
			return "", ErrConflict
		}
		if user != "" && user != *link {
			_, err = tx.Exec(ctx, `UPDATE login_transactions SET phase='conflict' WHERE id=$1`, id)
			return "conflict", err
		}
		user = *link
		if _, err = tx.Exec(ctx, `UPDATE users SET kind='registered' WHERE id=$1`, user); err != nil {
			return "", err
		}
	} else if user == "" {
		user = randomID()
		if _, err = tx.Exec(ctx, `INSERT INTO users(id,name,kind) VALUES($1,$2,'registered')`, user, name); err != nil {
			return "", err
		}
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT state='active' FROM users WHERE id=$1`, user).Scan(&active); err != nil {
		return "", err
	}
	if !active {
		return "", ErrForbidden
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_identities(issuer,subject,user_id) VALUES($1,$2,$3) ON CONFLICT(issuer,subject) DO NOTHING`, issuer, subject, user); err != nil {
		return "", err
	}
	if link == nil {
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM user_devices WHERE user_id=$1 AND NOT revoked AND (expires_at IS NULL OR expires_at>now())`, user).Scan(&count); err != nil {
			return "", err
		}
		if count >= 10 {
			return "", ErrConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO devices(id,name,public_key) VALUES($1,$2,$3)`, device, name, pub); err != nil {
			return "", ErrConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO user_devices(device_id,user_id,expires_at) VALUES($1,$2,now()+interval '30 days')`, device, user); err != nil {
			return "", err
		}
	} else {
		if _, err = tx.Exec(ctx, `UPDATE user_devices SET expires_at=now()+interval '30 days' WHERE device_id=$1`, device); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE device_id=$1 AND scope='player'`, device); err != nil {
			return "", err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE login_transactions SET phase='ready',result_user_id=$2 WHERE id=$1`, id, user); err != nil {
		return "", err
	}
	err = adminEvent(ctx, tx, "user:"+user, "user.oidc_login", user, map[string]string{"device_id": device})
	return "ready", err
}

func (s *Server) oidcConfirm(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(r) {
		s.fail(w, ErrForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if r.ParseForm() != nil {
		s.fail(w, ErrInvalid)
		return
	}
	id := r.PostForm.Get("id")
	if len(id) != 32 {
		s.fail(w, ErrInvalid)
		return
	}
	cookie, err := r.Cookie(loginCookie(id))
	if err != nil || subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(hash("confirm:"+cookie.Value))) != 1 {
		s.fail(w, ErrForbidden)
		return
	}
	phase := ""
	err = s.Store.Write(r.Context(), func(tx pgx.Tx) error {
		var browser string
		if e := tx.QueryRow(r.Context(), `SELECT browser_hash FROM login_transactions WHERE id=$1`, id).Scan(&browser); e != nil {
			return ErrForbidden
		}
		if subtle.ConstantTimeCompare([]byte(browser), []byte(hash(cookie.Value))) != 1 {
			return ErrForbidden
		}
		var e error
		phase, e = completeIdentity(r.Context(), tx, id)
		return e
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	message := "账号已授权，请返回客户端。"
	if phase == "conflict" {
		message = "该身份已经绑定其他账号。访客数据未合并，请返回客户端选择登录已有账号。"
	}
	renderLogin(w, message, "", "", "", "")
}

func (s *Server) oidcClaim(w http.ResponseWriter, r *http.Request) {
	var in model.LoginClaim
	if err := decodeRequest(w, r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if len(in.ID) != 32 || len(in.Proof) != 64 {
		s.fail(w, ErrInvalid)
		return
	}
	if err := s.Store.Rate(r.Context(), "oidc-claim:"+requestIP(r), 240, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	out := model.LoginResult{}
	err := s.Store.Write(r.Context(), func(tx pgx.Tx) error {
		var device, proof string
		var pub []byte
		err := tx.QueryRow(r.Context(), `SELECT device_id,proof_hash,public_key,phase FROM login_transactions WHERE id=$1 AND expires_at>now() AND consumed_at IS NULL AND config_revision=(SELECT revision FROM oidc_settings WHERE id=1 AND enabled)`, in.ID).Scan(&device, &proof, &pub, &out.State)
		if err != nil {
			return ErrForbidden
		}
		if subtle.ConstantTimeCompare([]byte(proof), []byte(hash(in.Proof))) != 1 || !ed25519.Verify(pub, []byte("nodelane-login-claim:"+in.ID+":"+in.Proof), in.Signature) {
			return ErrForbidden
		}
		if out.State != "ready" {
			return nil
		}
		session, err := issuePlayerSession(r.Context(), tx, device)
		if err != nil {
			return err
		}
		out.Session = &session
		_, err = tx.Exec(r.Context(), `UPDATE login_transactions SET consumed_at=now(),phase='consumed' WHERE id=$1`, in.ID)
		return err
	})
	s.result(w, out, err)
}
