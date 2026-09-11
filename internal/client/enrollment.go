package client

import (
	"context"
	"crypto/ed25519"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (a *API) NodeBinding() (string, int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.session.NodeID, a.session.Generation
}

// Enroll authenticates a pre-authorized registration without creating a player
// identity. The caller must persist Identity before this request.
func (a *API) Enroll(ctx context.Context, key string) error {
	pub := ed25519.PrivateKey(a.Identity.PrivateKey).Public().(ed25519.PublicKey)
	var challenge model.Challenge
	in := model.EnrollmentChallengeRequest{Key: key, ChallengeRequest: model.ChallengeRequest{DeviceID: a.Identity.ID(), Name: a.Identity.Name, PublicKey: pub}}
	if err := a.request(ctx, "POST", "/v2/node/enrollment/challenge", in, &challenge, "", ID()); err != nil {
		return err
	}
	sig := ed25519.Sign(a.Identity.PrivateKey, append([]byte("nodelane-auth-v2:enrollment:"+challenge.ID+":"), challenge.Nonce...))
	a.mu.Lock()
	defer a.mu.Unlock()
	err := a.request(ctx, "POST", "/v2/node/enrollment/complete", model.VerifyRequest{ID: challenge.ID, Signature: sig}, &a.session, "", ID())
	if err == nil {
		a.terminal = nil
	}
	return err
}
