package agent

import (
	"context"
	"encoding/json"
	"github.com/nodelane/nodelane-room/internal/localapi"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (r *Runtime) Init(ctx context.Context, server, name string) (string, error) {
	r.op.Lock()
	defer r.op.Unlock()
	if r.api != nil {
		return "", localapi.Failure("already_initialized", "device is already initialized")
	}
	i, err := device.NewIdentity(server, name)
	if err != nil {
		return "", localapi.Failure("invalid_request", err.Error())
	}
	a := client.NewAPI(i)
	if err = a.Authenticate(ctx); err != nil {
		return "", err
	}
	if err = r.persist(i); err != nil {
		return "", err
	}
	r.api = a
	r.Wake()
	return i.ID(), nil
}

func (r *Runtime) Action(ctx context.Context, action, room string, body json.RawMessage) (any, error) {
	r.op.Lock()
	defer r.op.Unlock()
	if r.api == nil {
		return nil, localapi.Failure("unconfigured", "run nlroom-cli init first")
	}
	if r.identity.Node {
		return nil, localapi.Failure("forbidden", "infrastructure identity cannot operate player rooms")
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
	if room == "" && action != "create" && action != "join" {
		return nil, localapi.Failure("no_room", "no selected room")
	}
	var out json.RawMessage
	if err := r.api.Call(ctx, "POST", path, body, &out); err != nil {
		return nil, err
	}
	i := r.identity
	if action == "create" || action == "join" {
		var result model.RoomResult
		if err := json.Unmarshal(out, &result); err != nil {
			return nil, err
		}
		i.RoomID = result.Room.ID
		if err := r.persist(i); err != nil {
			return nil, err
		}
		r.netMu.Lock()
		r.stopNetworkLocked()
		r.netMu.Unlock()
	}
	if (action == "leave" || action == "close") && room == i.RoomID {
		r.netMu.Lock()
		r.stopNetworkLocked()
		r.netMu.Unlock()
		i.RoomID = ""
		if err := r.persist(i); err != nil {
			return nil, err
		}
	}
	r.Wake()
	return out, nil
}

func (r *Runtime) Members(ctx context.Context, room string) (model.Snapshot, error) {
	r.op.Lock()
	defer r.op.Unlock()
	var s model.Snapshot
	if r.api == nil {
		return s, localapi.Failure("unconfigured", "run init first")
	}
	if room == "" {
		room = r.identity.RoomID
	}
	if room == "" {
		return s, localapi.Failure("no_room", "no selected room")
	}
	err := r.api.Call(ctx, "GET", "/v2/rooms/"+room, nil, &s)
	return s, err
}

func (r *Runtime) Games(ctx context.Context) ([]model.Game, error) {
	r.op.Lock()
	defer r.op.Unlock()
	if r.api == nil {
		return nil, localapi.Failure("unconfigured", "run init first")
	}
	var out []model.Game
	err := r.api.Call(ctx, "GET", "/v2/games", nil, &out)
	return out, err
}

func (r *Runtime) OwnedRooms(ctx context.Context) ([]model.Room, error) {
	r.op.Lock()
	api := r.api
	r.op.Unlock()
	if api == nil {
		return nil, localapi.Failure("unconfigured", "请先初始化设备")
	}
	var out []model.Room
	err := api.Call(ctx, "GET", "/v2/rooms", nil, &out)
	return out, err
}

func (r *Runtime) ManageRoom(ctx context.Context, room string) (model.RoomManagement, error) {
	var out model.RoomManagement
	if !validLocalID(room, false) {
		return out, localapi.Failure("invalid_request", "invalid room ID")
	}
	r.op.Lock()
	api := r.api
	r.op.Unlock()
	if api == nil {
		return out, localapi.Failure("unconfigured", "请先初始化设备")
	}
	err := api.Call(ctx, "GET", "/v2/rooms/"+room+"/manage", nil, &out)
	return out, err
}
