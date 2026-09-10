package agent

import (
	"context"
	"errors"
	"math/rand/v2"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/engine"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
	"github.com/nodelane/nodelane-room/internal/probe"
	"github.com/slackhq/nebula/cert"
)

func (r *Runtime) step(ctx context.Context) error {
	if r.api == nil || (r.nodeMode && r.identity.NodeID == "") {
		return nil
	}
	i := r.identity
	if i.RoomID == "" && !i.Node {
		r.netMu.Lock()
		r.stopNetworkLocked()
		r.snapshot = model.Snapshot{}
		r.netMu.Unlock()
		if r.watchCancel != nil {
			r.watchCancel()
			r.watchCancel = nil
			r.watchRoom = ""
		}
		r.setError("idle", nil)
		return nil
	}
	if !i.Node && r.watchRoom != i.RoomID {
		if r.watchCancel != nil {
			r.watchCancel()
		}
		watchCtx, cancel := context.WithCancel(ctx)
		r.watchCancel = cancel
		r.watchRoom = i.RoomID
		api, room := r.api, i.RoomID
		go func() {
			var rev int64
			for watchCtx.Err() == nil {
				_ = api.Watch(watchCtx, room, rev, func(s model.Snapshot) {
					if s.Room != nil {
						rev = s.Room.Revision
					}
					r.Wake()
				})
				timer := time.NewTimer(2 * time.Second)
				select {
				case <-watchCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}()
	}
	prefix := "/v2/rooms/" + i.RoomID
	var snapshot model.Snapshot
	if i.Node {
		prefix = "/v2/node"
		var allowed bool
		var err error
		snapshot, allowed, err = r.prepareNode(ctx)
		if err != nil {
			return err
		}
		if !allowed {
			return nil
		}
	} else {
		if err := r.api.Call(ctx, "POST", prefix+"/heartbeat", struct{}{}, nil); err != nil {
			if client.IsDenied(err) {
				r.netMu.Lock()
				r.stopNetworkLocked()
				r.snapshot = model.Snapshot{}
				r.netMu.Unlock()
				// A denied heartbeat is authoritative: the device no longer has
				// room membership. Clear the local selection so it can join again.
				i.RoomID, i.Ports = "", nil
				if saveErr := r.persist(i); saveErr != nil {
					return saveErr
				}
				r.setError("connected", err)
				return nil
			}
			return err
		}
		if err := r.api.Call(ctx, "GET", prefix, nil, &snapshot); err != nil {
			return err
		}
	}
	if delta := time.Since(snapshot.ServerTime); delta > 30*time.Second || delta < -30*time.Second {
		return errors.New("system clock differs from control server by more than 30 seconds")
	}
	// Apply revocations immediately, even when the following renewal request
	// fails. A successful snapshot must not leave obsolete permissions active.
	r.netMu.Lock()
	if r.engine.Running() {
		current := engine.Config{Lease: r.lease, Snapshot: snapshot, PrivateKey: r.key, DeviceID: i.ID(), Interface: "nodelane0", RelayIPs: r.selectRelaysLocked(snapshot.Nodes)}
		if err := r.engine.Apply(current); err != nil {
			r.stopNetworkLocked()
			r.netMu.Unlock()
			return err
		}
		if r.engine.Generation() != r.engineGeneration {
			r.closeAdaptersLocked()
		}
		r.snapshot = snapshot
		if r.probe != nil {
			r.probe.Update(snapshot)
		}
	}
	r.netMu.Unlock()
	r.netMu.Lock()
	lease := r.lease
	renew := lease.IP == "" || (!r.renewAt.IsZero() && !time.Now().Before(r.renewAt)) || (r.renewAt.IsZero() && time.Until(lease.ExpiresAt) < 7*time.Minute)
	key, pub := r.key, r.public
	r.netMu.Unlock()
	if renew {
		if len(key) == 0 {
			var err error
			key, pub, err = pki.TunnelKey()
			if err != nil {
				return err
			}
		}
		if err := r.api.Call(ctx, "POST", prefix+"/lease", model.LeaseRequest{PublicKey: pub, Revision: r.nodeRevision()}, &lease); err != nil {
			if client.IsDenied(err) {
				r.netMu.Lock()
				r.stopNetworkLocked()
				r.netMu.Unlock()
			}
			return err
		}
	}
	if i.Node && r.nodeSelected != nil {
		n := *r.nodeSelected
		lease.Node = &n
	}
	cfg := engine.Config{Lease: lease, Snapshot: snapshot, PrivateKey: key, DeviceID: i.ID(), Interface: "nodelane0"}
	if err := engine.Validate(cfg); err != nil {
		r.netMu.Lock()
		r.stopNetworkLocked()
		r.netMu.Unlock()
		return err
	}
	ca, _, err := cert.UnmarshalCertificateFromPEM([]byte(lease.CA))
	if err != nil {
		return err
	}
	fp, err := ca.Fingerprint()
	if err != nil {
		return err
	}
	if i.CAFingerprint != "" && i.CAFingerprint != fp {
		return errors.New("control server changed the pinned Nebula CA")
	}
	if i.CAFingerprint == "" {
		i.CAFingerprint = fp
		if err = r.persist(i); err != nil {
			return err
		}
	}
	r.netMu.Lock()
	cfg.RelayIPs = r.selectRelaysLocked(snapshot.Nodes)
	if err = r.engine.Apply(cfg); err != nil {
		r.netMu.Unlock()
		return err
	}
	if r.engine.Generation() != r.engineGeneration {
		r.closeAdaptersLocked()
	}
	r.engineGeneration = r.engine.Generation()
	if i.Node && r.udpTraffic == nil {
		r.udpTraffic = startUDPTraffic(r.nodeState.ListenPort)
	}
	r.lease = lease
	if renew {
		r.renewAt = lease.ExpiresAt.Add(-7 * time.Minute).Add(time.Duration(rand.IntN(15)) * time.Second)
		r.nodeLastRenewal = time.Now().UTC()
	}
	r.key = key
	r.public = pub
	r.snapshot = snapshot
	if r.probe == nil {
		r.probe, err = probe.Start(lease.IP, lease.Network, i.Node)
		if err != nil {
			r.stopNetworkLocked()
			r.netMu.Unlock()
			return err
		}
	}
	r.probe.Update(snapshot)
	p := r.probe
	members := append([]model.Member(nil), snapshot.Members...)
	nodes := append([]model.Node(nil), snapshot.Nodes...)
	r.netMu.Unlock()
	if i.Node {
		if err = r.nodeApplied(); err != nil {
			return err
		}
	}
	r.stateMu.Lock()
	r.status.Control = "connected"
	r.status.Error = r.nodePending
	r.stateMu.Unlock()
	if r.probeBusy.CompareAndSwap(false, true) {
		go func() {
			defer r.probeBusy.Store(false)
			var wg sync.WaitGroup
			ips := map[string]bool{}
			for _, m := range members {
				if m.DeviceID != i.ID() {
					ips[m.IP] = true
				}
			}
			for _, n := range nodes {
				if n.DeviceID != i.ID() {
					ips[n.IP] = true
				}
			}
			if i.Node {
				observed, _ := r.engine.Network()
				for _, peer := range observed.Peers {
					if len(ips) < 128 {
						ips[peer.IP] = true
					}
				}
			}
			for ip := range ips {
				wg.Add(1)
				go func(ip string) { defer wg.Done(); _ = r.engine.Connect(ip); _, _ = p.Ping(ctx, ip) }(ip)
			}
			wg.Wait()
		}()
	}
	if !i.Node {
		ports := []model.EndpointRequest{}
		if snapshot.Room != nil && snapshot.Room.Game == "custom" {
			ports = append(ports, i.Ports...)
		}
		for _, e := range ports {
			k := e.Protocol + ":" + strconv.Itoa(int(e.Port))
			if time.Since(r.registered[k]) < 10*time.Second {
				continue
			}
			if err = r.api.Call(ctx, "POST", prefix+"/endpoints", e, nil); err != nil {
				return err
			}
			r.registered[k] = time.Now()
		}
	}
	return nil
}

func (r *Runtime) selectRelaysLocked(nodes []model.Node) []string {
	type candidate struct {
		ip      string
		latency float64
	}
	candidates := []candidate{}
	healthy := map[string]bool{}
	for _, n := range nodes {
		if !n.Relay || n.Draining || n.DeviceID == r.identity.ID() {
			continue
		}
		latency := 1e9
		if r.probe != nil {
			rtt, _ := r.probe.Stats(n.IP)
			if rtt != nil {
				latency = *rtt
			}
			if r.probe.Failed(n.IP) {
				continue
			}
		}
		candidates = append(candidates, candidate{n.IP, latency})
		healthy[n.IP] = true
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].latency == candidates[j].latency {
			return candidates[i].ip < candidates[j].ip
		}
		return candidates[i].latency < candidates[j].latency
	})
	keep := len(r.relays) > 0
	for _, ip := range r.relays {
		if !healthy[ip] {
			keep = false
		}
	}
	if keep && time.Since(r.relaySelected) < time.Minute {
		return r.relays
	}
	if keep && len(candidates) > 0 && r.probe != nil {
		current, _ := r.probe.Stats(r.relays[0])
		if current != nil && candidates[0].latency >= *current*0.8 {
			r.relaySelected = time.Now()
			return r.relays
		}
	}
	r.relays = []string{}
	for _, c := range candidates {
		if len(r.relays) == 2 {
			break
		}
		r.relays = append(r.relays, c.ip)
	}
	r.relaySelected = time.Now()
	return r.relays
}

func (r *Runtime) stopNetworkLocked() {
	r.engine.Stop()
	r.closeAdaptersLocked()
	r.lease = model.Lease{}
	r.key = nil
	r.public = nil
	r.relays = nil
}

func (r *Runtime) closeAdaptersLocked() {
	r.udpTraffic.Close()
	r.udpTraffic = nil
	if r.probe != nil {
		r.probe.Close()
		r.probe = nil
	}
}
