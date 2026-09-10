package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (r *Runtime) Init(ctx context.Context, server, name string) (string, error) {
	r.op.Lock()
	defer r.op.Unlock()
	if r.api != nil {
		return "", errors.New("device is already initialized")
	}
	i, err := device.NewIdentity(server, name)
	if err != nil {
		return "", err
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
		return nil, errors.New("run nodelane init first")
	}
	if r.identity.Node {
		return nil, errors.New("infrastructure identity cannot operate player rooms")
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
		return nil, errors.New("no selected room")
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
		i.Ports = nil
		r.registered = map[string]time.Time{}
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
		i.Ports = nil
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
		return s, errors.New("run init first")
	}
	if room == "" {
		room = r.identity.RoomID
	}
	if room == "" {
		return s, errors.New("no selected room")
	}
	err := r.api.Call(ctx, "GET", "/v2/rooms/"+room, nil, &s)
	return s, err
}

func (r *Runtime) Games(ctx context.Context) ([]model.Game, error) {
	r.op.Lock()
	defer r.op.Unlock()
	if r.api == nil {
		return nil, errors.New("run init first")
	}
	var out []model.Game
	err := r.api.Call(ctx, "GET", "/v2/games", nil, &out)
	return out, err
}

func (r *Runtime) SetPort(ctx context.Context, in model.EndpointRequest, remove bool) error {
	r.op.Lock()
	defer r.op.Unlock()
	if r.identity.RoomID == "" {
		return errors.New("join a room first")
	}
	if (in.Protocol != "tcp" && in.Protocol != "udp") || in.Port == 0 || in.Port == model.ProbePort {
		return errors.New("invalid game port")
	}
	var snap model.Snapshot
	if r.api == nil {
		return errors.New("run init first")
	}
	if err := r.api.Call(ctx, "GET", "/v2/rooms/"+r.identity.RoomID, nil, &snap); err != nil {
		return err
	}
	if snap.Room == nil || snap.Room.Game != "custom" {
		return errors.New("自定义端口仅适用于通用游戏；当前游戏端口由服务端配置")
	}
	i := r.identity
	i.Ports = append([]model.EndpointRequest(nil), i.Ports...)
	for index, p := range i.Ports {
		if p.Protocol == in.Protocol && p.Port == in.Port {
			if !remove {
				return nil
			}
			i.Ports = append(i.Ports[:index], i.Ports[index+1:]...)
			break
		}
	}
	if !remove && len(i.Ports) >= 32 {
		return errors.New("at most 32 explicit ports are supported")
	}
	if !remove {
		i.Ports = append(i.Ports, in)
	}
	// Persist removal first so the run loop cannot renew a removed permission,
	// including after a restart or an interrupted DELETE. Failed deletes expire.
	if err := r.persist(i); err != nil {
		return err
	}
	if remove {
		r.registered = map[string]time.Time{}
		if err := r.api.Call(ctx, "DELETE", "/v2/rooms/"+i.RoomID+"/endpoints", in, nil); err != nil {
			r.Wake()
			return err
		}
	}
	r.Wake()
	return nil
}
