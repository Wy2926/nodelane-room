package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/localapi"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
)

type pendingLogin struct {
	Identity device.Identity    `json:"identity"`
	Attempt  model.LoginAttempt `json:"attempt"`
	Proof    string             `json:"proof"`
	Link     bool               `json:"link"`
}

func (r *Runtime) accountAction(ctx context.Context, in localapi.Request) (any, error) {
	r.op.Lock()
	defer r.op.Unlock()
	switch in.Action {
	case "account-login", "account-link":
		link := in.Action == "account-link"
		var i device.Identity
		var api *client.API
		var err error
		if link {
			if r.api == nil {
				return nil, localapi.Failure("unconfigured", "create a guest first")
			}
			i = r.identity
			api = r.api
		} else {
			server, name := in.Server, in.Name
			if r.identity.Server != "" {
				server = r.identity.Server
				if name == "" {
					name = r.identity.Name
				}
			}
			i, err = device.NewIdentity(server, name)
			if err != nil {
				return nil, err
			}
			api = client.NewAPI(i)
		}
		proof := client.ID() + client.ID()
		attempt, err := api.BeginLogin(ctx, proof, link)
		if err != nil {
			return nil, err
		}
		pending := pendingLogin{Identity: i, Attempt: attempt, Proof: proof, Link: link}
		b, err := json.Marshal(pending)
		if err != nil {
			return nil, err
		}
		if err = platform.SavePrivateFile(filepath.Join(r.dir, "login.bin"), b, true); err != nil {
			return nil, err
		}
		return attempt, nil
	case "account-poll":
		b, err := platform.LoadPrivateFile(filepath.Join(r.dir, "login.bin"))
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{"state": "none"}, nil
		}
		if err != nil {
			return nil, err
		}
		var pending pendingLogin
		if err = json.Unmarshal(b, &pending); err != nil {
			return nil, err
		}
		if time.Now().After(pending.Attempt.ExpiresAt) {
			return map[string]string{"state": "expired"}, nil
		}
		api := client.NewAPI(pending.Identity)
		out, err := api.ClaimLogin(ctx, pending.Attempt.ID, pending.Proof)
		// A successful server claim may lose its HTTP reply. The newly authorized
		// device key recovers the same account; it never registers a new guest.
		if err != nil {
			if e := api.Authenticate(ctx); e != nil {
				return nil, err
			}
			if pending.Link && (api.Account() == nil || api.Account().Kind != "registered") {
				return nil, err
			}
			out.State = "ready"
		}
		if out.State != "ready" {
			return map[string]string{"state": out.State}, nil
		}
		if !pending.Link && r.api != nil && r.identity.RoomID != "" {
			if err = r.api.Call(ctx, "POST", "/v2/rooms/"+r.identity.RoomID+"/leave", model.MemberRequest{}, nil); err != nil && !client.IsDenied(err) {
				return nil, err
			}
		}
		i := pending.Identity
		if pending.Link {
			i.RoomID = r.identity.RoomID
			i.CAFingerprint = r.identity.CAFingerprint
		}
		if err = r.persist(i); err != nil {
			return nil, err
		}
		if !pending.Link {
			r.netMu.Lock()
			r.stopNetworkLocked()
			r.snapshot = model.Snapshot{}
			r.netMu.Unlock()
		}
		r.api = api
		r.stateMu.Lock()
		r.status.User = api.Account()
		if r.status.User != nil {
			r.status.Name = r.status.User.Name
		}
		r.stateMu.Unlock()
		if err = r.clearLogin(); err != nil {
			return nil, err
		}
		r.Wake()
		return map[string]string{"state": "ready"}, nil
	case "account-cancel":
		err := r.clearLogin()
		return map[string]bool{"ok": err == nil}, err
	case "account-logout", "account-takeover":
		if r.api == nil {
			return nil, localapi.Failure("unconfigured", "no account")
		}
		path := "/v2/me/takeover"
		if in.Action == "account-logout" {
			if err := r.clearLogin(); err != nil {
				return nil, err
			}
			path = "/v2/me/logout"
		}
		if err := r.api.Call(ctx, "POST", path, struct{}{}, nil); err != nil {
			// A lost logout response can be followed by a denied retry: the
			// revoked key must still lead to a durable local signed-out state.
			if in.Action != "account-logout" || !client.IsDenied(err) {
				return nil, err
			}
		}
		if in.Action == "account-logout" {
			r.netMu.Lock()
			r.stopNetworkLocked()
			r.snapshot = model.Snapshot{}
			r.netMu.Unlock()
			if r.watchCancel != nil {
				r.watchCancel()
				r.watchCancel = nil
				r.watchRoom = ""
			}
			i := r.identity
			i.RoomID = ""
			i.SignedOut = true
			if err := r.persist(i); err != nil {
				return nil, err
			}
			r.api = nil
			r.stateMu.Lock()
			r.status.User = nil
			r.status.Control = "signed_out"
			r.stateMu.Unlock()
		}
		r.Wake()
		return map[string]bool{"ok": true}, nil
	}
	return nil, localapi.Failure("unknown_action", "unknown account action")
}

func (r *Runtime) clearLogin() error {
	err := os.Remove(filepath.Join(r.dir, "login.bin"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
