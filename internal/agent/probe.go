package agent

import (
	"context"
	"sync"
	"time"
)

// Sampling belongs to the service, independent of GUI polling and control requests.
func (r *Runtime) runProbes(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.probePeers(ctx)
		}
	}
}

func (r *Runtime) probePeers(ctx context.Context) {
	r.netMu.Lock()
	now := time.Now()
	if ctx.Err() != nil || r.probe == nil || !r.engine.Running() || !now.Before(r.lease.ExpiresAt) || (!r.nodeMode && (r.snapshot.Self.ValidUntil == nil || !now.Before(*r.snapshot.Self.ValidUntil))) {
		r.netMu.Unlock()
		return
	}
	p := r.probe
	ips := map[string]bool{}
	for _, m := range r.snapshot.Members {
		if m.IP != r.lease.IP {
			ips[m.IP] = true
		}
	}
	for _, n := range r.snapshot.Nodes {
		if n.IP != r.lease.IP {
			ips[n.IP] = true
		}
	}
	if r.nodeMode {
		observed, _ := r.engine.Network()
		for _, peer := range observed.Peers {
			if len(ips) < 128 && peer.IP != r.lease.IP {
				ips[peer.IP] = true
			}
		}
	}
	r.netMu.Unlock()
	var wg sync.WaitGroup
	for ip := range ips {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.engine.Connect(ip)
			_, _ = p.Ping(ctx, ip)
		}()
	}
	wg.Wait()
}
