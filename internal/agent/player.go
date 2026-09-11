package agent

import (
	"context"
	"encoding/json"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/localapi"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (r *Runtime) Init(ctx context.Context, server, name string) (string, error) {
	r.op.Lock()
	defer r.op.Unlock()
	if r.api != nil && !r.identity.PendingGuest {
		return "", localapi.Failure("local_already_initialized", "device is already initialized")
	}
	i, err := device.NewIdentity(server, name)
	if r.identity.PendingGuest && r.identity.Server == server && r.identity.Name == name {
		i = r.identity
		err = nil
	}
	if err != nil {
		return "", err
	}
	if err = r.clearLogin(); err != nil {
		return "", err
	}
	i.PendingGuest = true
	if err = r.persist(i); err != nil {
		return "", err
	}
	a := client.NewAPI(i)
	if err = a.Authenticate(ctx); err != nil {
		return "", err
	}
	i.PendingGuest = false
	if err = r.persist(i); err != nil {
		return "", err
	}
	r.api = a
	r.stateMu.Lock()
	r.status.User = a.Account()
	r.stateMu.Unlock()
	r.Wake()
	return i.ID(), nil
}

func (r *Runtime) Action(ctx context.Context, action, room string, body json.RawMessage) (any, error) {
	r.op.Lock()
	defer r.op.Unlock()
	if r.api == nil {
		return nil, localapi.Failure("local_unconfigured", "run nlroom-cli init first")
	}
	if r.identity.Node {
		return nil, localapi.Failure("resource_not_found", "infrastructure identity cannot operate player rooms")
	}
	if (action == "create" || action == "join" || action == "owner-join") && (requiredLocally(r.updateStatus().Policy) || r.updateStatus().State == "installing") {
		return nil, localapi.Failure("client_update_required", "client update required")
	}
	if room == "" {
		room = r.identity.RoomID
	}
	path := "/v2/rooms/" + room + "/" + action
	if action == "create" {
		path = "/v2/rooms"
	}
	if action == "join" {
		path = "/v2/rooms/join"
	}
	if action == "owner-join" {
		path = "/v2/rooms/" + room + "/join"
	}
	if action == "invite-revoke" {
		path = "/v2/rooms/" + room + "/invite/revoke"
	}
	if action == "create" || action == "join" || action == "owner-join" {
		r.reportVersion(ctx)
	}
	if room == "" && action != "create" && action != "join" {
		return nil, localapi.Failure("local_no_room", "no selected room")
	}
	result, err := r.api.CallResult(ctx, "POST", path, body)
	if err != nil {
		return nil, err
	}
	if err := r.applyActionResult(ctx, action, path, result.Data); err != nil {
		return nil, &model.BusinessError{Code: model.Code(err), Details: model.Details{KnownCommit: true}}
	}
	r.Wake()
	return result, nil
}

func (r *Runtime) applyActionResult(ctx context.Context, action, path string, data json.RawMessage) error {
	switch action {
	case "create", "join", "owner-join", "leave", "close":
	default:
		return nil
	}
	var me model.AccountStatus
	if err := r.api.Call(ctx, "GET", "/v2/me", nil, &me); err != nil {
		return err
	}
	i := r.identity
	i.RoomID = ""
	if me.Membership.State == "active" {
		i.RoomID = me.Membership.RoomID
	}
	if i.RoomID != r.identity.RoomID {
		r.netMu.Lock()
		r.stopNetworkLocked()
		r.snapshot = model.Snapshot{Self: me.Membership}
		r.netMu.Unlock()
		if err := r.persist(i); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) Members(ctx context.Context, room string) (model.Snapshot, error) {
	r.op.Lock()
	defer r.op.Unlock()
	var s model.Snapshot
	if r.api == nil {
		return s, localapi.Failure("local_unconfigured", "run init first")
	}
	if room == "" {
		room = r.identity.RoomID
	}
	if room == "" {
		return s, localapi.Failure("local_no_room", "no selected room")
	}
	err := r.api.Call(ctx, "GET", "/v2/rooms/"+room, nil, &s)
	return s, err
}

func (r *Runtime) Games(ctx context.Context) ([]model.Game, error) {
	r.op.Lock()
	defer r.op.Unlock()
	if r.api == nil {
		return nil, localapi.Failure("local_unconfigured", "run init first")
	}
	var out []model.Game
	err := r.api.Call(ctx, "GET", "/v2/games", nil, &out)
	return out, err
}

func (r *Runtime) OwnedRooms(ctx context.Context) (model.RoomPage, error) {
	r.op.Lock()
	api := r.api
	r.op.Unlock()
	if api == nil {
		return model.RoomPage{}, localapi.Failure("local_unconfigured", "请先初始化设备")
	}
	var out model.RoomPage
	err := api.Call(ctx, "GET", "/v2/rooms", nil, &out)
	return out, err
}

func (r *Runtime) ManageRoom(ctx context.Context, room string) (model.RoomManagement, error) {
	var out model.RoomManagement
	if !validLocalID(room, false) {
		return out, localapi.Failure("request_validation_failed", "invalid room ID")
	}
	r.op.Lock()
	api := r.api
	r.op.Unlock()
	if api == nil {
		return out, localapi.Failure("local_unconfigured", "请先初始化设备")
	}
	err := api.Call(ctx, "GET", "/v2/rooms/"+room+"/manage", nil, &out)
	return out, err
}
