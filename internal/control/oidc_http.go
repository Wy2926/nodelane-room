package control

import (
	"context"
	"crypto/subtle"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Server) registerOIDC(mux *http.ServeMux) {
	mux.HandleFunc("POST /v2/auth/oidc/start", func(w http.ResponseWriter, r *http.Request) { s.oidcStart(w, r, "") })
	mux.HandleFunc("POST /v2/me/identity", s.playerAuth(s.oidcStart))
	mux.HandleFunc("POST /v2/auth/oidc/claim", s.oidcClaim)
	mux.HandleFunc("GET /v2/auth/oidc/browser", s.oidcBrowser)
	mux.HandleFunc("GET /v2/auth/oidc/callback", s.oidcCallback)
	mux.HandleFunc("POST /v2/auth/oidc/confirm", s.oidcConfirm)
	mux.HandleFunc("GET /v2/admin/oidc", s.adminHandler(s.adminOIDCSettings))
	mux.HandleFunc("PUT /v2/admin/oidc", s.adminWrite(s.adminMutation(s.adminOIDC)))
}

func (s *Server) adminOIDCSettings(w http.ResponseWriter, r *http.Request, _ string) {
	out, err := s.Store.oidcSettings(r.Context())
	s.result(w, out, err)
}

func (s *Server) adminOIDC(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
	var in model.OIDCSettings
	if err := decodeBytes(b, &in); err != nil {
		return nil, err
	}
	return saveOIDC(r.Context(), tx, actor, in)
}

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request, device string) {
	var in model.LoginStart
	if err := decodeRequest(w, r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if err := validateLoginStart(in, device); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.Rate(r.Context(), "oidc-start:"+requestIP(r), 30, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	out, err := s.startLogin(r.Context(), device, hash(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), in)
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
	address, browser, err := s.authorizeLogin(ctx, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: loginCookie(id), Value: browser, Path: "/v2/auth/oidc/", HttpOnly: true, Secure: strings.HasPrefix(s.PublicURL, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: 300})
	http.Redirect(w, r, address, http.StatusSeeOther)
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
	a, err := s.Store.loginAuthorization(ctx, state)
	if err != nil {
		s.fail(w, err)
		return
	}
	cookie, err := r.Cookie(loginCookie(a.ID))
	if err != nil {
		s.fail(w, model.Failure("auth_login_validation_failed"))
		return
	}
	err = s.completeLogin(ctx, a, cookie.Value, r.URL.Query().Get("code"), r.URL.Query().Get("error") != "")
	if model.Code(err) == "auth_login_denied" {
		w.WriteHeader(http.StatusForbidden)
		renderLogin(w, "登录未完成，请返回客户端重试。", "", "", "", "")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	renderLogin(w, "身份验证成功，请确认授权设备。", a.ID, a.Name, a.Device[:12], hash("confirm:"+cookie.Value))
}

func (s *Server) oidcConfirm(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(r) {
		s.fail(w, model.Failure("request_origin_rejected"))
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
		s.fail(w, model.Failure("auth_login_validation_failed"))
		return
	}
	phase, err := s.Store.confirmLogin(r.Context(), id, cookie.Value)
	if err != nil {
		s.fail(w, err)
		return
	}
	message := "账号已授权，请返回客户端。"
	if phase != "ready" && phase != "consumed" {
		result := model.NewResult(phase, "control", "", nil)
		w.WriteHeader(model.HTTPStatus(result.Code))
		message = result.Message
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
	out, err := s.Store.claimLogin(r.Context(), in)
	s.result(w, out, err)
}
