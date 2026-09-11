package engine

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"

	"github.com/nodelane/nodelane-room/internal/lan"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/slackhq/nebula/config"
	"github.com/slackhq/nebula/overlay"
	"github.com/slackhq/nebula/routing"
)

type ethernetDevice struct {
	*lan.Device
	networks []netip.Prefix
	name     string
	activate func() error
}

func (d *ethernetDevice) Activate() error                       { return d.activate() }
func (d *ethernetDevice) Networks() []netip.Prefix              { return d.networks }
func (d *ethernetDevice) Name() string                          { return d.name }
func (d *ethernetDevice) RoutesFor(netip.Addr) routing.Gateways { return routing.Gateways{} }
func (d *ethernetDevice) SupportsMultiqueue() bool              { return false }
func (d *ethernetDevice) NewMultiQueueReader() (io.ReadWriteCloser, error) {
	return nil, errors.New("LAN uses one bounded TAP queue")
}

func isPlayer(c Config) bool {
	return c.Lease.Node == nil
}
func interfaceName(c Config) string {
	if isPlayer(c) {
		return "nodelane0-lan"
	}
	if c.Interface != "" {
		return c.Interface
	}
	return "nodelane0"
}

func (e *Engine) lanFactory(c Config) overlay.DeviceFactory {
	return func(_ *config.C, _ *slog.Logger, networks []netip.Prefix, _ int) (overlay.Device, error) {
		if len(networks) != 1 {
			return nil, errors.New("LAN requires one certificate network")
		}
		open := e.tapFactory
		if open == nil {
			open = lan.OpenTAP
		}
		tap, err := open(interfaceName(c), networks[0])
		if err != nil {
			return nil, err
		}
		d, err := lan.New(tap, tap.MAC, networks[0], c.Lease, c.Snapshot)
		if err != nil {
			tap.Close()
			return nil, err
		}
		e.lan = d
		return &ethernetDevice{d, networks, tap.Name, tap.Activate}, nil
	}
}

func (e *Engine) LANMAC() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lan != nil {
		return e.lan.MAC()
	}
	return ""
}

func (e *Engine) ProbeConn() net.PacketConn {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lan != nil {
		return e.lan.ProbeConn()
	}
	return nil
}

func (e *Engine) LANStatus() *model.LANStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lan == nil {
		return nil
	}
	s := e.lan.Status()
	s.Interface = interfaceName(e.applied)
	return &s
}
