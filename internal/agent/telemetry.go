package agent

import (
	"context"
	"sort"
	"time"
)

// Called under op, with a short independent request deadline. Reports use a
// dedicated endpoint so no observations enter durable idempotency records.
func (r *Runtime) reportTelemetry(ctx context.Context) {
	if time.Since(r.telemetryAt) < 5*time.Second {
		return
	}
	r.netMu.Lock()
	if !r.engine.Running() {
		r.netMu.Unlock()
		return
	}
	sample, iface := r.engine.Network()
	if r.identity.Node {
		sample.Traffic = r.udpTraffic.Sample()
	} else {
		sample.Traffic = interfaceTraffic(iface)
	}
	// Rotate bounded node observations; connection count covers the full hostmap.
	if len(sample.Peers) > 128 {
		sort.Slice(sample.Peers, func(i, j int) bool { return sample.Peers[i].DeviceID < sample.Peers[j].DeviceID })
		start := r.telemetryOffset % len(sample.Peers)
		peers := append(sample.Peers[start:], sample.Peers[:start]...)
		sample.Peers = peers[:128]
		r.telemetryOffset = start + 128
	}
	for i := range sample.Peers {
		p := &sample.Peers[i]
		if r.probe != nil {
			p.RTTMillis, p.LossPercent = r.probe.Stats(p.IP)
			p.ProbeAt = r.probe.LastSample(p.IP)
		}
	}
	r.netMu.Unlock()
	r.telemetryAt = time.Now()
	path := "/v2/rooms/" + r.identity.RoomID + "/telemetry"
	if r.identity.Node {
		path = "/v2/node/telemetry"
	}
	request, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	// Monitoring failure must not revoke otherwise valid control authorization.
	_ = r.api.Call(request, "POST", path, sample, nil)
}
