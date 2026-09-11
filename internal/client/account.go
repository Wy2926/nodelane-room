package client

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"

	"github.com/nodelane/nodelane-room/internal/model"
)

func (a *API) Capabilities(ctx context.Context) (model.Capabilities, error) {
	var out model.Capabilities
	err := a.request(ctx, "GET", "/v2/capabilities", nil, &out, "", "")
	if err == nil && (out.Contract != model.Contract || out.LANVersion != model.LANVersion) {
		err = model.Failure("api_contract_unsupported")
	}
	return out, err
}

func (a *API) BeginLogin(ctx context.Context, proof string, link bool) (model.LoginAttempt, error) {
	pub := ed25519.PrivateKey(a.Identity.PrivateKey).Public().(ed25519.PublicKey)
	h := sha256.Sum256([]byte(proof))
	digest := hex.EncodeToString(h[:])
	in := model.LoginStart{DeviceID: a.Identity.ID(), PublicKey: pub, Name: a.Identity.Name, Proof: digest}
	in.Signature = ed25519.Sign(a.Identity.PrivateKey, []byte("nodelane-login-start:"+in.DeviceID+":"+digest))
	var out model.LoginAttempt
	var err error
	if link {
		err = a.Call(ctx, "POST", "/v2/me/identity", in, &out)
	} else {
		err = a.request(ctx, "POST", "/v2/auth/oidc/start", in, &out, "", "")
	}
	if err != nil {
		return out, err
	}
	u, e := url.Parse(out.URL)
	base, _ := url.Parse(a.Identity.Server)
	if e != nil || u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil || u.Path != "/v2/auth/oidc/browser" || u.Fragment != "" || u.RawQuery != "id="+out.ID || len(out.ID) != 32 {
		return model.LoginAttempt{}, errors.New("invalid login URL")
	}
	return out, nil
}

func (a *API) ClaimLogin(ctx context.Context, id, proof string) (model.LoginResult, error) {
	in := model.LoginClaim{ID: id, Proof: proof, Signature: ed25519.Sign(a.Identity.PrivateKey, []byte("nodelane-login-claim:"+id+":"+proof))}
	var out model.LoginResult
	err := a.request(ctx, "POST", "/v2/auth/oidc/claim", in, &out, "", "")
	if err == nil && out.Session != nil {
		a.mu.Lock()
		a.session = *out.Session
		a.terminal = nil
		a.mu.Unlock()
	}
	return out, err
}

func (a *API) RefreshAccount(ctx context.Context) (model.AccountStatus, error) {
	var me model.AccountStatus
	if err := a.Call(ctx, "GET", "/v2/me", nil, &me); err != nil {
		return model.AccountStatus{}, err
	}
	a.mu.Lock()
	u := me.User
	a.session.User = &u
	a.mu.Unlock()
	return me, nil
}
