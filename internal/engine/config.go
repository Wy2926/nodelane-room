package engine

import (
	"errors"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
	"github.com/slackhq/nebula/cert"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Lease      model.Lease
	Snapshot   model.Snapshot
	PrivateKey []byte
	DeviceID   string
	RelayIPs   []string
	Interface  string
	ListenPort int
	ListenHost string
	DisableTUN bool
}

func Validate(c Config) error {
	if c.Lease.Node == nil {
		if c.Snapshot.Game == nil {
			return errors.New("LAN game policy missing")
		}
		if err := model.ValidateLAN(*c.Snapshot.Game); err != nil {
			return err
		}
	}
	if c.Lease.IP == "" || time.Until(c.Lease.ExpiresAt) <= 0 {
		return errors.New("lease missing or expired")
	}
	leaf, _, err := cert.UnmarshalCertificateFromPEM([]byte(c.Lease.Certificate))
	if err != nil {
		return err
	}
	ca, _, err := cert.UnmarshalCertificateFromPEM([]byte(c.Lease.CA))
	if err != nil {
		return err
	}
	if !ca.IsCA() || ca.Expired(time.Now()) || !ca.CheckSignature(ca.PublicKey()) || !leaf.CheckSignature(ca.PublicKey()) || leaf.Expired(time.Now()) || leaf.Name() != c.DeviceID || !leaf.NotAfter().Equal(c.Lease.ExpiresAt) {
		return errors.New("lease certificate validation failed")
	}
	if err = leaf.VerifyPrivateKey(cert.Curve_CURVE25519, c.PrivateKey); err != nil {
		return err
	}
	network, err := netip.ParsePrefix(c.Lease.Network)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(c.Lease.IP)
	if err != nil || !network.Contains(ip) {
		return errors.New("lease IP outside overlay")
	}
	if len(leaf.Networks()) != 1 || leaf.Networks()[0] != netip.PrefixFrom(ip, network.Bits()) {
		return errors.New("certificate address does not match lease")
	}
	expected := "room:" + c.Lease.RoomID
	if c.Lease.Node != nil {
		expected = "infrastructure"
	}
	if len(leaf.Groups()) != 1 || leaf.Groups()[0] != expected {
		return errors.New("certificate room group mismatch")
	}
	if c.Lease.Node == nil {
		if c.Snapshot.Room == nil || c.Snapshot.Room.Closed || c.Snapshot.Room.ID != c.Lease.RoomID {
			return errors.New("room unavailable")
		}
		member := false
		for _, m := range c.Snapshot.Members {
			if m.DeviceID == c.DeviceID && m.IP == c.Lease.IP {
				member = true
			}
		}
		if !member {
			return errors.New("device is no longer a room member")
		}
	}
	return nil
}

func Render(c Config) (string, error) {
	if err := Validate(c); err != nil {
		return "", err
	}
	static := map[string][]string{}
	lighthouses := []string{}
	relays := []string{}
	for _, n := range c.Snapshot.Nodes {
		if n.DeviceID == c.DeviceID {
			continue
		}
		static[n.IP] = []string{n.Address}
		if n.Lighthouse && !n.Draining {
			lighthouses = append(lighthouses, n.IP)
		}
		if n.Relay && !n.Draining {
			relays = append(relays, n.IP)
		}
	}
	if c.RelayIPs != nil {
		allowed := map[string]bool{}
		for _, ip := range relays {
			allowed[ip] = true
		}
		relays = nil
		for _, ip := range c.RelayIPs {
			if allowed[ip] {
				relays = append(relays, ip)
			}
		}
	}
	if len(relays) > 2 {
		relays = relays[:2]
	}
	sort.Strings(lighthouses)
	lighthouse, relay := false, false
	listenPort := c.ListenPort
	if c.Lease.Node != nil {
		lighthouse = c.Lease.Node.Lighthouse
		relay = c.Lease.Node.Relay
		_, port, e := net.SplitHostPort(c.Lease.Node.Address)
		if e != nil {
			return "", e
		}
		listenPort, e = strconv.Atoi(port)
		if e != nil {
			return "", e
		}
	}
	if lighthouse {
		lighthouses = []string{}
	}
	group := "room:" + c.Lease.RoomID
	inbound := []map[string]any{}
	outbound := []map[string]any{}
	rule := func(proto string, port any, group string) map[string]any {
		return map[string]any{"proto": proto, "port": port, "group": group}
	}
	if c.Lease.Node != nil {
		inbound = append(inbound, map[string]any{"proto": "udp", "port": model.ProbePort, "host": "any"})
		outbound = append(outbound, map[string]any{"proto": "udp", "port": model.ProbePort, "host": "any"})
	} else {
		for _, g := range []string{group, "infrastructure"} {
			inbound = append(inbound, rule("udp", model.ProbePort, g))
			outbound = append(outbound, rule("udp", model.ProbePort, g))
		}
		inbound = append(inbound, rule("icmp", "any", group))
		outbound = append(outbound, rule("icmp", "any", group))
		inbound = append(inbound, rule("udp", model.LANPort, group))
		outbound = append(outbound, rule("udp", model.LANPort, group))
	}
	dev := interfaceName(c)
	listenHost := c.ListenHost
	if listenHost == "" {
		listenHost = "0.0.0.0"
	}
	v := map[string]any{
		"pki":             map[string]any{"ca": c.Lease.CA, "cert": c.Lease.Certificate, "key": pki.PrivatePEM(c.PrivateKey), "blocklist": c.Snapshot.Blocklist, "disconnect_invalid": true},
		"static_host_map": static,
		"lighthouse":      map[string]any{"am_lighthouse": lighthouse, "hosts": lighthouses, "interval": 10},
		"listen":          map[string]any{"host": listenHost, "port": listenPort, "windows_bypass_wdf": !c.DisableTUN},
		"punchy":          map[string]any{"punch": true, "respond": true},
		"relay":           map[string]any{"am_relay": relay, "use_relays": !relay, "relays": relays},
		"tun":             map[string]any{"dev": dev, "disabled": c.DisableTUN, "mtu": 1300, "drop_local_broadcast": true, "drop_multicast": true, "network_category": "private", "windows_bypass_wdf": true},
		"firewall":        map[string]any{"inbound_action": "drop", "outbound_action": "drop", "inbound": inbound, "outbound": outbound},
		"logging":         map[string]any{"level": "info", "format": "json"},
	}
	if isPlayer(c) {
		v["nodelane_lan"] = map[string]any{"network": c.Snapshot.Game.Network, "ports": c.Snapshot.Game.Ports, "enabled": c.Snapshot.Game.Enabled}
	}
	b, err := yaml.Marshal(v)
	return string(b), err
}

func CheckAddressConflict(network string, ownInterface string) error {
	n, err := netip.ParsePrefix(network)
	if err != nil {
		return err
	}
	ifs, err := net.Interfaces()
	if err != nil {
		return err
	}
	for _, iface := range ifs {
		if iface.Name == ownInterface || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, e := iface.Addrs()
		if e != nil {
			return e
		}
		for _, a := range addrs {
			p, e := netip.ParsePrefix(a.String())
			if e == nil && p.Addr().Is4() && (n.Contains(p.Addr()) || p.Contains(n.Addr())) {
				return model.Failure("local_route_conflict")
			}
		}
	}
	return checkRoutes(n, ownInterface)
}
