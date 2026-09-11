package control

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
	"golang.org/x/oauth2"
)

func validIssuer(issuer string) bool {
	u, e := url.Parse(issuer)
	return e == nil && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")))
}

func saveOIDC(ctx context.Context, tx pgx.Tx, actor string, in model.OIDCSettings) (any, error) {
	if !validIssuer(in.Issuer) || len(in.Issuer) > 2048 || !model.ValidLabel(in.ClientID, 256) || len(in.ClientSecret) > 4096 {
		return nil, ErrInvalid
	}
	var revision int64
	if err := tx.QueryRow(ctx, "SELECT COALESCE((SELECT revision FROM oidc_settings WHERE id=1),0)").Scan(&revision); err != nil {
		return nil, err
	}
	if in.Revision != revision {
		return nil, model.RevisionError(in.Revision, revision)
	}
	// Retain an omitted secret only for the same issuer and client.
	_, err := tx.Exec(ctx, `INSERT INTO oidc_settings(id,issuer,client_id,client_secret,enabled) VALUES(1,$1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET issuer=$1,client_id=$2,client_secret=CASE WHEN $3='' AND oidc_settings.issuer=$1 AND oidc_settings.client_id=$2 THEN oidc_settings.client_secret ELSE $3 END,enabled=$4,revision=oidc_settings.revision+1`, in.Issuer, in.ClientID, in.ClientSecret, in.Enabled)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE login_transactions SET phase='auth_login_config_changed',nonce=NULL,verifier=NULL,state_hash=NULL,issuer=NULL,subject=NULL WHERE consumed_at IS NULL`)
	}
	if err == nil {
		err = adminEvent(ctx, tx, actor, "oidc.configured", "oidc", map[string]any{"issuer": in.Issuer, "client_id": in.ClientID, "enabled": in.Enabled})
	}
	return map[string]bool{"ok": err == nil}, err
}

func (s *Store) oidcSettings(ctx context.Context) (model.OIDCSettings, error) {
	var out model.OIDCSettings
	err := s.Pool.QueryRow(ctx, `SELECT issuer,client_id,enabled,revision FROM oidc_settings WHERE id=1`).Scan(&out.Issuer, &out.ClientID, &out.Enabled, &out.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return out, err
}

func (s *Server) oidcConfig(ctx context.Context) (model.OIDCSettings, int64, error) {
	var c model.OIDCSettings
	var rev int64
	err := s.Store.Pool.QueryRow(ctx, `SELECT issuer,client_id,client_secret,enabled,revision FROM oidc_settings WHERE id=1 AND enabled`).Scan(&c.Issuer, &c.ClientID, &c.ClientSecret, &c.Enabled, &rev)
	if err == pgx.ErrNoRows {
		err = model.Failure("auth_oidc_unavailable")
	}
	return c, rev, err
}

func (s *Server) oidcClient(ctx context.Context, c model.OIDCSettings) (*oidc.Provider, oauth2.Config, error) {
	provider, err := oidc.NewProvider(ctx, c.Issuer)
	if err != nil {
		return nil, oauth2.Config{}, model.Failure("auth_login_provider_unavailable")
	}
	return provider, oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: strings.TrimRight(s.PublicURL, "/") + "/v2/auth/oidc/callback", Scopes: []string{oidc.ScopeOpenID, "profile"}}, nil
}

func validateLoginStart(in model.LoginStart, device string) error {
	proof, e := hex.DecodeString(in.Proof)
	if e != nil || len(proof) != 32 || len(in.PublicKey) != ed25519.PublicKeySize || deviceID(in.PublicKey) != in.DeviceID || !model.ValidLabel(in.Name, 80) || !ed25519.Verify(in.PublicKey, []byte("nodelane-login-start:"+in.DeviceID+":"+in.Proof), in.Signature) {
		return ErrInvalid
	}
	if device != "" && device != in.DeviceID {
		return model.Failure("auth_login_validation_failed")
	}
	return nil
}

func (s *Server) startLogin(ctx context.Context, device, sessionHash string, in model.LoginStart) (model.LoginAttempt, error) {
	_, rev, err := s.oidcConfig(ctx)
	if err != nil {
		return model.LoginAttempt{}, err
	}
	if s.PublicURL == "" {
		return model.LoginAttempt{}, ErrInvalid
	}
	out := model.LoginAttempt{ID: hash(in.DeviceID + ":" + in.Proof)[:32], ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}
	out.URL = s.PublicURL + "/v2/auth/oidc/browser?id=" + out.ID
	err = s.Store.Write(ctx, func(tx pgx.Tx) error {
		var user, session *string
		if device != "" {
			h := sessionHash
			if e := validPlayerSession(ctx, tx, device, h); e != nil {
				return e
			}
			u, e := playerUser(ctx, tx, device)
			if e != nil {
				return e
			}
			if u.Kind != "guest" {
				return model.Failure("account_link_not_allowed")
			}
			user = &u.ID
			session = &h
		} else {
			var exists bool
			if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE id=$1)`, in.DeviceID).Scan(&exists); e != nil {
				return e
			}
			if exists {
				return model.Failure("account_link_not_allowed")
			}
		}
		_, e := tx.Exec(ctx, `INSERT INTO login_transactions(id,device_id,public_key,name,proof_hash,link_user_id,session_hash,config_revision,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(id) DO NOTHING`, out.ID, in.DeviceID, in.PublicKey, in.Name, in.Proof, user, session, rev, out.ExpiresAt)
		if e != nil {
			return e
		}
		return tx.QueryRow(ctx, "SELECT expires_at FROM login_transactions WHERE id=$1", out.ID).Scan(&out.ExpiresAt)
	})
	return out, err
}

func (s *Server) authorizeLogin(ctx context.Context, id string) (string, string, error) {
	c, rev, err := s.oidcConfig(ctx)
	if err != nil {
		return "", "", err
	}
	_, config, err := s.oidcClient(ctx, c)
	if err != nil {
		return "", "", err
	}
	browser, state, nonce, verifier := randomID()+randomID(), randomID()+randomID(), randomID()+randomID(), oauth2.GenerateVerifier()
	tag, err := s.Store.Pool.Exec(ctx, `UPDATE login_transactions SET browser_hash=$2,state_hash=$3,nonce=$4,verifier=$5,phase='authorizing' WHERE id=$1 AND phase='pending' AND expires_at>now() AND config_revision=$6`, id, hash(browser), hash(state), nonce, verifier, rev)
	if err != nil {
		return "", "", err
	}
	if tag.RowsAffected() != 1 {
		return "", "", ErrConflict
	}
	return config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce)), browser, nil
}

type loginAuthorization struct {
	ID, Browser, Nonce, Verifier, Name, Device string
	Revision                                   int64
}

func (s *Store) loginAuthorization(ctx context.Context, state string) (loginAuthorization, error) {
	var a loginAuthorization
	err := s.Pool.QueryRow(ctx, `SELECT id,browser_hash,nonce,verifier,name,device_id,config_revision FROM login_transactions WHERE state_hash=$1 AND phase='authorizing' AND expires_at>now()`, hash(state)).Scan(&a.ID, &a.Browser, &a.Nonce, &a.Verifier, &a.Name, &a.Device, &a.Revision)
	if err != nil {
		return a, model.Failure("auth_login_validation_failed")
	}
	return a, nil
}

func (s *Server) completeLogin(ctx context.Context, a loginAuthorization, browserToken, code string, denied bool) error {
	if subtle.ConstantTimeCompare([]byte(hash(browserToken)), []byte(a.Browser)) != 1 {
		return model.Failure("auth_login_validation_failed")
	}
	id, nonce, verifier, revision := a.ID, a.Nonce, a.Verifier, a.Revision
	// Claim the callback before exchanging its one-use authorization code.
	tag, err := s.Store.Pool.Exec(ctx, `UPDATE login_transactions SET phase='exchanging',verifier=NULL WHERE id=$1 AND phase='authorizing'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	success := false
	failureCode := "auth_login_validation_failed"
	defer func() {
		if !success {
			_, _ = s.Store.Pool.Exec(context.WithoutCancel(ctx), `UPDATE login_transactions SET phase=$2,nonce=NULL,verifier=NULL WHERE id=$1 AND phase='exchanging'`, id, failureCode)
		}
	}()
	if denied {
		failureCode = "auth_login_denied"
		return model.Failure(failureCode)
	}
	c, rev, err := s.oidcConfig(ctx)
	if err != nil || rev != revision {
		failureCode = "auth_login_config_changed"
		return model.Failure(failureCode)
	}
	provider, config, err := s.oidcClient(ctx, c)
	if err != nil {
		return err
	}
	token, err := config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return model.Failure("auth_login_validation_failed")
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return model.Failure("auth_login_validation_failed")
	}
	verified, err := provider.Verifier(&oidc.Config{ClientID: c.ClientID}).Verify(ctx, raw)
	if err != nil || verified.Nonce != nonce || verified.Subject == "" || len(verified.Subject) > 255 {
		return model.Failure("auth_login_validation_failed")
	}
	var claims struct {
		AZP string `json:"azp"`
	}
	if verified.Claims(&claims) != nil || (claims.AZP != "" && claims.AZP != c.ClientID) || (len(verified.Audience) > 1 && claims.AZP == "") {
		return model.Failure("auth_login_validation_failed")
	}
	tag, err = s.Store.Pool.Exec(ctx, `UPDATE login_transactions SET phase='verified',issuer=$2,subject=$3,nonce=NULL WHERE id=$1 AND phase='exchanging' AND expires_at>now() AND config_revision=(SELECT revision FROM oidc_settings WHERE id=1 AND enabled)`, id, verified.Issuer, verified.Subject)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	success = true
	return nil
}

// Linking preserves the guest's user ID. Existing identities never merge users.
func completeIdentity(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	var device, name, issuer, subject string
	var pub []byte
	var link, session *string
	err := tx.QueryRow(ctx, `SELECT device_id,name,public_key,issuer,subject,link_user_id,session_hash FROM login_transactions WHERE id=$1 AND phase='verified' AND expires_at>now() AND config_revision=(SELECT revision FROM oidc_settings WHERE id=1 AND enabled)`, id).Scan(&device, &name, &pub, &issuer, &subject, &link, &session)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", model.Failure("auth_login_unusable")
	}
	if err != nil {
		return "", err
	}
	var user string
	err = tx.QueryRow(ctx, `SELECT user_id FROM user_identities WHERE issuer=$1 AND subject=$2`, issuer, subject).Scan(&user)
	if err != nil && err != pgx.ErrNoRows {
		return "", err
	}
	if link != nil {
		if session == nil {
			return "", model.Failure("auth_login_unusable")
		}
		if err = validPlayerSession(ctx, tx, device, *session); err != nil {
			return "", err
		}
		u, e := playerUser(ctx, tx, device)
		if e != nil {
			return "", e
		}
		if u.ID != *link || u.Kind != "guest" {
			return "", model.Failure("account_link_not_allowed")
		}
		if user != "" && user != *link {
			_, err = tx.Exec(ctx, `UPDATE login_transactions SET phase='account_identity_conflict' WHERE id=$1`, id)
			return "account_identity_conflict", err
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
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM users WHERE id=$1`, user).Scan(&state); err != nil {
		return "", err
	}
	if state == "deleted" {
		return "", model.Failure("account_deleted")
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
			return "", model.Failure("account_device_limit")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO devices(id,name,public_key) VALUES($1,$2,$3)`, device, name, pub); err != nil {
			return "", model.Failure("account_link_not_allowed")
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

func (s *Store) confirmLogin(ctx context.Context, id, browserToken string) (string, error) {
	phase := ""
	err := s.Write(ctx, func(tx pgx.Tx) error {
		var browser string
		if e := tx.QueryRow(ctx, `SELECT browser_hash FROM login_transactions WHERE id=$1`, id).Scan(&browser); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return model.Failure("auth_login_validation_failed")
			}
			return e
		}
		if subtle.ConstantTimeCompare([]byte(browser), []byte(hash(browserToken))) != 1 {
			return model.Failure("auth_login_validation_failed")
		}
		if e := tx.QueryRow(ctx, "SELECT phase FROM login_transactions WHERE id=$1", id).Scan(&phase); e != nil {
			return e
		}
		if phase == "ready" || phase == "consumed" {
			return nil
		}
		if phase != "verified" {
			return model.Failure("auth_login_unusable")
		}
		attempt, e := tx.Begin(ctx)
		if e != nil {
			return e
		}
		phase, e = completeIdentity(ctx, attempt, id)
		if e == nil {
			return attempt.Commit(ctx)
		}
		if re := attempt.Rollback(ctx); re != nil {
			return re
		}
		var failure *model.BusinessError
		if !errors.As(e, &failure) {
			return e
		}
		phase = failure.Code
		_, e = tx.Exec(ctx, "UPDATE login_transactions SET phase=$2,nonce=NULL,verifier=NULL WHERE id=$1", id, phase)
		return e
	})
	return phase, err
}

func (s *Store) claimLogin(ctx context.Context, in model.LoginClaim) (model.LoginResult, error) {
	out := model.LoginResult{}
	err := s.Write(ctx, func(tx pgx.Tx) error {
		var device, proof string
		var pub []byte
		var expires time.Time
		var consumed *time.Time
		err := tx.QueryRow(ctx, `SELECT device_id,proof_hash,public_key,phase,expires_at,consumed_at FROM login_transactions WHERE id=$1`, in.ID).Scan(&device, &proof, &pub, &out.State, &expires, &consumed)
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Failure("auth_login_unusable")
		}
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(proof), []byte(hash(in.Proof))) != 1 || !ed25519.Verify(pub, []byte("nodelane-login-claim:"+in.ID+":"+in.Proof), in.Signature) {
			return model.Failure("auth_proof_invalid")
		}
		if !expires.After(time.Now()) {
			return model.Failure("auth_login_expired")
		}
		if consumed != nil {
			return model.Failure("auth_login_unusable")
		}
		out.Code = "auth_login_pending"
		if out.State == "verified" {
			out.Code = "auth_login_confirmation_required"
		}
		if _, ok := model.BusinessCodes[out.State]; ok {
			out.Code = out.State
			out.State = "failed"
		}
		if out.State != "ready" {
			return nil
		}
		session, err := issuePlayerSession(ctx, tx, device)
		if err != nil {
			return err
		}
		out.Session = &session
		out.Code = "ok"
		_, err = tx.Exec(ctx, `UPDATE login_transactions SET consumed_at=now(),phase='consumed' WHERE id=$1`, in.ID)
		return err
	})
	return out, err
}
