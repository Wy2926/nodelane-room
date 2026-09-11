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
		a.mu.Unlock()
	}
	return out, err
}

func (a *API) RefreshAccount(ctx context.Context) (*model.User, error) {
	var u model.User
	if err := a.Call(ctx, "GET", "/v2/me", nil, &u); err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.session.User = &u
	a.mu.Unlock()
	return &u, nil
}
