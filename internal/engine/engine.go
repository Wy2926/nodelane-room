package engine

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/slackhq/nebula"
	"github.com/slackhq/nebula/cert"
	"github.com/slackhq/nebula/config"
	"github.com/slackhq/nebula/overlay"
)

type Engine struct {
	mu            sync.Mutex
	control       *nebula.Control
	config        *config.C
	applied       Config
	raw           string
	log           *slog.Logger
	deviceFactory overlay.DeviceFactory
	generation    uint64
	epoch         string
}

func New(log *slog.Logger) *Engine {
	var epoch [16]byte
	_, _ = rand.Read(epoch[:])
	return &Engine{log: log, epoch: hex.EncodeToString(epoch[:])}
}

// NewWithDeviceFactory permits an upstream user-space device in integration
// tests. Production services call New and use Nebula's native OS TUN factory.
func NewWithDeviceFactory(log *slog.Logger, factory overlay.DeviceFactory) *Engine {
	e := New(log)
	e.deviceFactory = factory
	return e
}
func (e *Engine) Apply(c Config) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	raw, err := Render(c)
	if err != nil {
		return err
	}
	if raw == e.raw && e.control != nil && e.control.State() == nebula.StateStarted {
		return nil
	}
	next := config.NewC(e.log)
	if err = next.LoadString(raw); err != nil {
		return err
	}
	if e.control != nil && (e.applied.Lease.IP != c.Lease.IP || e.applied.Lease.RoomID != c.Lease.RoomID || e.control.State() != nebula.StateStarted || !reflect.DeepEqual(e.config.Get("firewall"), next.Get("firewall")) || !reflect.DeepEqual(e.config.Get("listen"), next.Get("listen")) || e.config.GetBool("lighthouse.am_lighthouse", false) != next.GetBool("lighthouse.am_lighthouse", false) || e.config.GetBool("relay.am_relay", false) != next.GetBool("relay.am_relay", false)) {
		// v1.11.1 assigns Interface.firewall without synchronization during reload.
		// Stop packet readers before changing rules; keep ordinary PKI/node reloads.
		e.stop()
	}
	if e.control == nil {
		if !c.DisableTUN {
			if err = CheckAddressConflict(c.Lease.Network, c.Interface); err != nil {
				return err
			}
		}
		cfg := config.NewC(e.log)
		if err = cfg.LoadString(raw); err != nil {
			return err
		}
		ctrl, err := nebula.Main(cfg, false, model.NebulaVersion, e.log, e.deviceFactory)
		if err != nil {
			return err
		}
		if err = ctrl.Start(); err != nil {
			ctrl.Stop()
			return err
		}
		e.control = ctrl
		e.config = cfg
		e.generation++
	} else {
		// Upstream reload callbacks may only log an invalid configuration. Validate
		// PKI and firewall rules first using the same upstream parsers.
		if err = preflight(raw, c.Lease.Certificate); err != nil {
			e.stop()
			return err
		}
		if err = e.config.ReloadConfigString(raw); err != nil {
			e.stop()
			return err
		}
		allowed := map[string]bool{}
		for _, m := range c.Snapshot.Members {
			allowed[m.IP] = true
		}
		for _, n := range c.Snapshot.Nodes {
			allowed[n.IP] = true
		}
		blocked := map[string]bool{}
		for _, f := range c.Snapshot.Blocklist {
			blocked[f] = true
		}
		relaysChanged := !slices.Equal(e.applied.RelayIPs, c.RelayIPs)
		for _, h := range e.control.ListHostmapHosts(false) {
			for _, ip := range h.VpnAddrs {
				invalid := c.Lease.Node == nil && !allowed[ip.String()]
				// Rebuild affected relay tunnels when health/drain selection changes.
				invalid = invalid || relaysChanged && !h.CurrentRemote.IsValid() && len(h.CurrentRelaysToMe) > 0
				if h.Cert != nil {
					fp, _ := h.Cert.Fingerprint()
					invalid = invalid || blocked[fp] || h.Cert.Expired(time.Now())
				}
				if invalid {
					e.control.CloseTunnel(ip, false)
				}
			}
		}
	}
	e.raw = raw
	e.applied = c
	return nil
}

func preflight(raw, certificate string) error {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.NewC(log)
	if err := cfg.LoadString(raw); err != nil {
		return err
	}
	if _, err := nebula.NewPKIFromConfig(log, cfg); err != nil {
		return err
	}
	leaf, _, err := cert.UnmarshalCertificateFromPEM([]byte(certificate))
	if err != nil {
		return err
	}
	fw := nebula.NewFirewall(log, 12*time.Minute, 3*time.Minute, 10*time.Minute, leaf)
	for _, inbound := range []bool{true, false} {
		if err = nebula.AddFirewallRulesFromConfig(log, inbound, cfg, fw); err != nil {
			return err
		}
	}
	return nil
}
func (e *Engine) stop() {
	if e.control != nil {
		e.control.Stop()
		_ = e.control.Wait()
		e.control = nil
		e.config = nil
	}
	e.raw = ""
}
func (e *Engine) Stop()              { e.mu.Lock(); defer e.mu.Unlock(); e.stop() }
func (e *Engine) Generation() uint64 { e.mu.Lock(); defer e.mu.Unlock(); return e.generation }
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.control != nil && e.control.State() == nebula.StateStarted
}
func (e *Engine) Connect(ip string) error {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.control == nil {
		return errors.New("Nebula is not running")
	}
	e.control.CreateTunnel(a)
	return nil
}
func (e *Engine) Close(ip string) {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.control != nil {
		e.control.CloseTunnel(a, false)
	}
}
func (e *Engine) Peers(members []model.Member) []model.Peer {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]model.Peer, 0, len(members))
	for _, m := range members {
		if m.DeviceID == e.applied.DeviceID {
			continue
		}
		p := model.Peer{DeviceID: m.DeviceID, Name: m.Name, IP: m.IP, Mode: "offline"}
		if e.control != nil {
			a, err := netip.ParseAddr(m.IP)
			if err == nil {
				h := e.control.GetHostInfoByVpnAddr(a, false)
				if h != nil {
					p.Mode = "unknown"
					if h.CurrentRemote.IsValid() {
						p.Mode = "direct"
					} else if len(h.CurrentRelaysToMe) > 0 {
						p.Mode = "relay"
					}
				} else if e.control.GetHostInfoByVpnAddr(a, true) != nil {
					p.Mode = "pending"
				}
			}
		}
		out = append(out, p)
	}
	return out
}

// Expiration checks are independent of control-plane polling and of Nebula's
// periodic connection checks, bounding offline authorization by the lease.
func (e *Engine) Expire(now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.control == nil {
		return
	}
	if !now.Before(e.applied.Lease.ExpiresAt) {
		e.stop()
		return
	}
	for _, h := range e.control.ListHostmapHosts(false) {
		if h.Cert != nil && h.Cert.Expired(now) {
			for _, ip := range h.VpnAddrs {
				e.control.CloseTunnel(ip, false)
			}
		}
	}
}

func (e *Engine) Path(ip string) (string, string) {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return "offline", ""
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.control == nil {
		return "offline", ""
	}
	h := e.control.GetHostInfoByVpnAddr(a, false)
	if h == nil {
		return "pending", ""
	}
	if h.CurrentRemote.IsValid() {
		return "direct", h.CurrentRemote.String()
	}
	if len(h.CurrentRelaysToMe) > 0 {
		return "relay", ""
	}
	return "unknown", ""
}
