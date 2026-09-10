package agent

import (
	"context"
	"errors"
	"time"

	"github.com/nodelane/nodelane-room/internal/localapi"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
)

func (r *Runtime) Status() model.Status {
	r.stateMu.Lock()
	s := r.status
	r.stateMu.Unlock()
	s.Version, s.ProtocolVersion = model.Version, localapi.ProtocolVersion
	r.netMu.Lock()
	defer r.netMu.Unlock()
	s.Room = r.snapshot.Room
	s.Game = r.snapshot.Game
	s.SnapshotAt = r.snapshot.ServerTime
	s.Members = append([]model.Member{}, r.snapshot.Members...)
	s.Endpoints = append([]model.Endpoint{}, r.snapshot.Endpoints...)
	s.IP = r.lease.IP
	s.LeaseExpiresAt = r.lease.ExpiresAt
	s.Engine = "stopped"
	if r.engine.Running() {
		s.Engine = "running"
	}
	s.Peers = r.engine.Peers(r.snapshot.Members)
	if r.probe != nil {
		for i := range s.Peers {
			s.Peers[i].RTTMillis, s.Peers[i].LossPercent = r.probe.Stats(s.Peers[i].IP)
		}
	}
	return s
}

func (r *Runtime) Ping(ctx context.Context, target string) (map[string]any, error) {
	r.netMu.Lock()
	p := r.probe
	ip := ""
	matches := 0
	for _, m := range r.snapshot.Members {
		if target == m.IP || target == m.DeviceID || target == m.Name {
			ip = m.IP
			matches++
		}
	}
	r.netMu.Unlock()
	if matches > 1 {
		return nil, errors.New("nickname is ambiguous; use device ID or virtual IP")
	}
	if p == nil || ip == "" {
		return nil, errors.New("connected room member not found")
	}
	_ = r.engine.Connect(ip)
	d, err := p.Ping(ctx, ip)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ip": ip, "rtt_ms": float64(d) / float64(time.Millisecond)}, nil
}

func (r *Runtime) Doctor() map[string]any {
	s := r.Status()
	result := map[string]any{"nebula_version": model.NebulaVersion, "device_id": s.DeviceID, "control": s.Control, "engine": s.Engine, "error": s.Error, "platform": platform.Diagnostics(), "peers": s.Peers}
	if s.IP != "" {
		result["lease_expires_at"] = s.LeaseExpiresAt
		result["virtual_ip"] = s.IP
	}
	return result
}
