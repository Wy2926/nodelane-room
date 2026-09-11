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
	Phase         string             `json:"phase"`
	LeaveID       string             `json:"leave_id"`
	LeaveDeadline time.Time          `json:"leave_deadline"`
	Identity      device.Identity    `json:"identity"`
	Attempt       model.LoginAttempt `json:"attempt"`
	Proof         string             `json:"proof"`
	Link          bool               `json:"link"`
}

func (r *Runtime) accountAction(ctx context.Context, in localapi.Request) (any, error) {
	r.op.Lock()
	defer r.op.Unlock()
	switch in.Action {
	case "account-login", "account-link":
		if b, err := platform.LoadPrivateFile(filepath.Join(r.dir, "login.bin")); err == nil {
			var pending pendingLogin
			if json.Unmarshal(b, &pending) != nil {
				return nil, model.Failure("local_storage_failed")
			}
			if pending.Phase == "waiting" && pending.Link == (in.Action == "account-link") && pending.Attempt.ExpiresAt.After(time.Now()) {
				return pending.Attempt, nil
			}
			return nil, model.Failure("local_busy")
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, model.Failure("local_storage_failed")
		}
		link := in.Action == "account-link"
		var i device.Identity
		var api *client.API
		var err error
		if link {
			if r.api == nil {
				return nil, localapi.Failure("local_unconfigured", "create a guest first")
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
		pending := pendingLogin{Identity: i, Proof: proof, Link: link, Phase: "starting"}
		if err = r.saveLogin(pending); err != nil {
			return nil, err
		}
		attempt, err := api.BeginLogin(ctx, proof, link)
		if err != nil {
			if loginTerminal(err) {
				if e := r.clearLogin(); e != nil {
					return nil, e
				}
			}
			return nil, err
		}
		pending.Attempt = attempt
		pending.Phase = "waiting"
		if err = r.saveLogin(pending); err != nil {
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
		api := client.NewAPI(pending.Identity)
		if pending.Link {
			api = r.api
			if api == nil {
				return nil, model.Failure("local_unconfigured")
			}
		}
		if pending.Attempt.ID == "" {
			attempt, e := api.BeginLogin(ctx, pending.Proof, pending.Link)
			if e != nil {
				state := "starting"
				if loginTerminal(e) {
					if err = r.clearLogin(); err != nil {
						return nil, err
					}
					state = "failed"
				}
				return map[string]string{"state": state, "code": model.Code(e)}, nil
			}
			pending.Attempt = attempt
			pending.Phase = "waiting"
			if e = r.saveLogin(pending); e != nil {
				return nil, e
			}
		}
		out, err := api.ClaimLogin(ctx, pending.Attempt.ID, pending.Proof)
		// A successful server claim may lose its HTTP reply. The newly authorized
		// device key recovers the same account; it never registers a new guest.
		if err != nil {
			if e := api.Reauthenticate(ctx); e != nil {
				if !pending.Attempt.ExpiresAt.After(time.Now()) {
					if e = r.clearLogin(); e != nil {
						return nil, e
					}
					return map[string]string{"state": "expired", "code": "auth_login_expired"}, nil
				}
				if loginTerminal(err) {
					if e = r.clearLogin(); e != nil {
						return nil, e
					}
					return map[string]string{"state": "failed", "code": model.Code(err)}, nil
				}
				return nil, err
			}
			if pending.Link && (api.Account() == nil || api.Account().Kind != "registered") {
				return nil, err
			}
			out.State = "ready"
		}
		if out.State != "ready" {
			if out.State == "failed" || out.State == "expired" {
				if err = r.clearLogin(); err != nil {
					return nil, err
				}
			}
			return map[string]string{"state": out.State, "code": out.Code}, nil
		}
		if !pending.Link && r.api != nil && r.identity.RoomID != "" {
			if pending.LeaveID == "" {
				pending.LeaveID = client.ID()
				pending.LeaveDeadline = time.Now().UTC().Add(time.Hour)
			}
			pending.Phase = "leaving_old_room"
			if err = r.saveLogin(pending); err != nil {
				return nil, err
			}
			if err = r.api.Call(client.WithOperation(ctx, pending.LeaveID, pending.LeaveDeadline), "POST", "/v2/rooms/"+r.identity.RoomID+"/leave", model.MemberRequest{}, nil); err != nil && !(client.MembershipEnded(err) || client.IdentityEnded(err)) {
				return nil, err
			}
		}
		i := pending.Identity
		if pending.Link {
			i.RoomID = r.identity.RoomID
			i.CAFingerprint = r.identity.CAFingerprint
		}
		if !pending.Link {
			r.netMu.Lock()
			r.stopNetworkLocked()
			r.snapshot = model.Snapshot{}
			r.netMu.Unlock()
		}
		pending.Phase = "saving_identity"
		if err = r.saveLogin(pending); err != nil {
			return nil, err
		}
		if err = r.persist(i); err != nil {
			return nil, err
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
		if _, err := os.Stat(filepath.Join(r.dir, "login.bin")); errors.Is(err, os.ErrNotExist) {
			return model.NewResult("operation_noop", "service", client.ID(), nil), nil
		}
		err := r.clearLogin()
		return map[string]string{"state": "cancelled", "code": "auth_login_cancelled"}, err
	case "account-logout", "account-takeover", "revoke-device":
		if r.api == nil {
			return nil, localapi.Failure("local_unconfigured", "no account")
		}
		path := "/v2/me/takeover"
		if in.Action == "revoke-device" {
			if !validLocalID(in.Target, false) {
				return nil, model.Validation("device_id", "invalid_format")
			}
			path = "/v2/me/devices/" + in.Target + "/revoke"
		}
		if in.Action == "account-logout" {
			if err := r.clearLogin(); err != nil {
				return nil, err
			}
			path = "/v2/me/logout"
		}
		if err := r.api.Call(ctx, "POST", path, in.Body, nil); err != nil {
			// A lost logout response can be followed by a denied retry: the
			// revoked key must still lead to a durable local signed-out state.
			if in.Action != "account-logout" || !model.IsCode(err, "auth_device_revoked") {
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
				return nil, &model.BusinessError{Code: "local_storage_failed", Details: model.Details{KnownCommit: true}}
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
	return nil, localapi.Failure("request_method_unsupported", "unknown account action")
}

func loginTerminal(err error) bool {
	return model.IsCode(err, "auth_oidc_unavailable", "auth_login_expired", "auth_login_unusable", "auth_login_denied", "auth_login_validation_failed", "auth_login_provider_unavailable", "auth_login_config_changed", "account_identity_conflict", "account_device_limit", "account_disabled", "account_deleted", "request_validation_failed")
}

func (r *Runtime) saveLogin(pending pendingLogin) error {
	b, err := json.Marshal(pending)
	if err != nil {
		return model.Failure("local_storage_failed")
	}
	if err = platform.SavePrivateFile(filepath.Join(r.dir, "login.bin"), b, true); err != nil {
		return model.Failure("local_storage_failed")
	}
	return nil
}

func (r *Runtime) clearLogin() error {
	err := os.Remove(filepath.Join(r.dir, "login.bin"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
