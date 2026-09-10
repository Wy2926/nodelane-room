package engine

import (
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

// Network only queries the pinned Nebula control API. Counts are established
// tunnels, including infrastructure; relay candidates never become active paths.
func (e *Engine) Network() (model.NetworkSample, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := model.NetworkSample{At: time.Now().UTC(), Epoch: e.epoch, Generation: e.generation, Peers: []model.LinkSample{}}
	if e.control == nil {
		return out, ""
	}
	seen := map[string]bool{}
	for _, h := range e.control.ListHostmapHosts(false) {
		if h.Cert == nil || h.Cert.Expired(out.At) || len(h.VpnAddrs) == 0 || seen[h.Cert.Name()] {
			continue
		}
		seen[h.Cert.Name()] = true
		p := model.LinkSample{DeviceID: h.Cert.Name(), IP: h.VpnAddrs[0].String(), Mode: "unknown", RelayIPs: []string{}}
		if h.CurrentRemote.IsValid() {
			p.Mode = "direct"
			p.Remote = h.CurrentRemote.String()
		} else if len(h.CurrentRelaysToMe) > 0 {
			p.Mode = "relay"
			for _, ip := range h.CurrentRelaysToMe {
				p.RelayIPs = append(p.RelayIPs, ip.String())
			}
		}
		out.Connections++
		out.Peers = append(out.Peers, p)
	}
	return out, e.control.Device().Name()
}
