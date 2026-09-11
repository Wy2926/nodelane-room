package agent

import (
	"context"
	"time"

	"github.com/nodelane/nodelane-room/internal/localapi"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
)

func (r *Runtime) Status() model.Status {
	r.stateMu.Lock()
	s := r.status
	s.Issues = append([]model.Issue{}, r.status.Issues...)
	r.stateMu.Unlock()
	s.ServiceInstanceID = r.instanceID
	s.Service = "ready"
	s.Identity = "active"
	switch s.Control {
	case "unconfigured":
		s.Identity = "unconfigured"
	case "signed_out":
		s.Identity = "signed_out"
	}
	if s.DeviceID != "" && s.User == nil && s.Identity == "active" {
		s.Identity = "authenticating"
	}
	for _, issue := range s.Issues {
		if issue.ResolvedAt == nil && model.EndsIdentity(issue.Code) {
			s.Identity = "reauth_required"
			if issue.Code == "account_disabled" || issue.Code == "account_deleted" {
				s.Identity = "blocked"
			}
		}
	}
	s.PendingOperations = r.pendingOperations()
	s.Operation = "idle"
	if len(s.PendingOperations) > 0 {
		s.Operation = s.PendingOperations[0].State
	}
	switch s.Control {
	case "connected", "idle":
		s.Control = "online"
	case "unreachable":
		s.Control = "offline"
	default:
		s.Control = "connecting"
	}
	s.Version, s.ProtocolVersion = model.ClientVersion, localapi.ProtocolVersion
	if r.nodeMode {
		s.Version = model.NodeVersion
	}
	s.LANVersion = model.LANVersion
	if !r.nodeMode {
		u := r.updateStatus()
		s.Update = &u
	}
	r.netMu.Lock()
	defer r.netMu.Unlock()
	s.Room = r.snapshot.Room
	s.Game = r.snapshot.Game
	s.SnapshotAt = r.snapshot.ServerTime
	s.Membership = r.snapshot.Self
	if s.Membership.State == "" {
		s.Membership.State = "none"
	}
	s.Permissions = r.snapshot.Permissions
	s.Members = append([]model.Member{}, r.snapshot.Members...)
	s.IP = r.lease.IP
	s.LeaseExpiresAt = r.lease.ExpiresAt
	s.LAN = r.engine.LANStatus()
	s.Network = model.NetworkState{State: "stopped", Generation: r.engine.Generation()}
	if s.SelectedRoom != "" {
		s.Network.State = "preparing"
	}
	if r.paused.Load() {
		s.Network.State = "suspended"
	}
	for _, issue := range s.Issues {
		if issue.ResolvedAt == nil && issue.Scope == "network" {
			s.Network.State = "failed"
			s.Network.Reason = issue.Code
		}
	}
	s.Engine = "stopped"
	if r.engine.Running() {
		s.Engine = "running"
		s.Network.State = "registering_mac"
		if s.Game != nil {
			s.Network.AppliedGameRevision = s.Game.Revision
		}
		if s.LAN != nil && s.LAN.Ready {
			s.Network.State = "ready"
		}
		if s.Membership.ValidUntil != nil && !time.Now().Before(*s.Membership.ValidUntil) {
			s.Network.State = "suspended"
			s.Network.Reason = "local_authorization_expired"
		}
		if s.Game != nil && !s.Game.Enabled {
			s.Network.State = "suspended"
			s.Network.Reason = "game_disabled"
		}
	}
	if s.Network.State == "registering_mac" {
		s.Network.Reason = "lan_mac_pending"
	}
	if s.Update != nil && s.Update.Required {
		s.Network.State = "suspended"
		s.Network.Reason = "client_update_required"
	}
	s.Peers = r.engine.Peers(r.snapshot.Members)
	if r.probe != nil {
		for i := range s.Peers {
			s.Peers[i].RTTMillis, s.Peers[i].LossPercent = r.probe.Stats(s.Peers[i].IP)
			s.Peers[i].MeasuredAt = r.probe.LastSample(s.Peers[i].IP)
		}
	}
	s.StatusSeq = r.statusSeq.Add(1)
	s.Freshness = model.Freshness{ObservedAt: time.Now().UTC(), SnapshotAt: s.SnapshotAt}
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
		return nil, model.Failure("local_probe_target_ambiguous")
	}
	if p == nil || ip == "" {
		return nil, model.Failure("local_probe_target_unavailable")
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
	result["lan_version"] = s.LANVersion
	if s.LAN != nil {
		result["lan"] = s.LAN
	}
	if s.IP != "" {
		result["lease_expires_at"] = s.LeaseExpiresAt
		result["virtual_ip"] = s.IP
	}
	return result
}
