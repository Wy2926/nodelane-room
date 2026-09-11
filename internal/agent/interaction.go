package agent

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/localapi"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
)

func (r *Runtime) networkAction(ctx context.Context, action string) (any, error) {
	if action == "network-stop" {
		r.pauseMu.Lock()
		defer r.pauseMu.Unlock()
		r.paused.Store(true)
		r.networkEpoch.Add(1)
		err := platform.SavePrivateFile(filepath.Join(r.dir, "network-paused.bin"), []byte("true"), true)
		r.netMu.Lock()
		r.stopNetworkLocked()
		r.netMu.Unlock()
		if err != nil {
			return nil, model.Failure("local_storage_failed")
		}
		return r.Status(), nil
	}
	if !r.op.TryLock() {
		return nil, model.Failure("local_busy")
	}
	defer r.op.Unlock()
	if r.api == nil {
		return nil, model.Failure("local_unconfigured")
	}
	epoch := r.networkEpoch.Load()
	var me model.AccountStatus
	if err := r.api.Call(ctx, "GET", "/v2/me", nil, &me); err != nil {
		return nil, err
	}
	if me.Membership.State != "active" || me.Membership.RoomID != r.identity.RoomID {
		return nil, model.Failure("local_no_room")
	}
	r.pauseMu.Lock()
	defer r.pauseMu.Unlock()
	if epoch != r.networkEpoch.Load() {
		return nil, model.Failure("local_request_cancelled")
	}
	if err := platform.SavePrivateFile(filepath.Join(r.dir, "network-paused.bin"), []byte("false"), true); err != nil {
		return nil, model.Failure("local_storage_failed")
	}
	r.netMu.Lock()
	r.stopNetworkLocked()
	r.netMu.Unlock()
	r.paused.Store(false)
	r.Wake()
	return r.Status(), nil
}

func (r *Runtime) readInteraction(ctx context.Context, in localapi.Request) (any, error) {
	r.op.Lock()
	api := r.api
	r.op.Unlock()
	if in.Action == "capabilities" {
		if api == nil {
			if device.ValidateURL(in.Server) != nil {
				return nil, model.Validation("server", "invalid_format")
			}
			api = client.NewAPI(device.Identity{Server: in.Server})
		}
		return api.Capabilities(ctx)
	}
	if api == nil {
		return nil, model.Failure("local_unconfigured")
	}
	path := "/v2/me/devices"
	if in.Action == "account-status" {
		path = "/v2/me"
	}
	if in.Action == "invite-info" {
		if !validLocalID(in.Room, false) {
			return nil, model.Validation("room", "invalid_format")
		}
		path = "/v2/rooms/" + in.Room + "/invite"
	}
	var out json.RawMessage
	err := api.Call(ctx, "GET", path, nil, &out)
	return out, err
}
