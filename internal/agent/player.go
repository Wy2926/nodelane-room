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

func (r *Runtime) AddPort(in model.EndpointRequest) error {
	r.op.Lock()
	defer r.op.Unlock()
	if r.identity.RoomID == "" {
		return errors.New("join a room first")
	}
	if (in.Protocol != "tcp" && in.Protocol != "udp") || in.Port == 0 || in.Port == model.ProbePort {
		return errors.New("invalid game port")
	}
	i := r.identity
	for _, p := range i.Ports {
		if p.Protocol == in.Protocol && p.Port == in.Port {
			return nil
		}
	}
	if len(i.Ports) >= 16 {
		return errors.New("at most 16 explicit ports are supported")
	}
	i.Ports = append(i.Ports, in)
	if err := r.persist(i); err != nil {
		return err
	}
	r.Wake()
	return nil
}
