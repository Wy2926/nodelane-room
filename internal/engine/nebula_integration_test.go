package engine

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
	"github.com/slackhq/nebula"
	"github.com/slackhq/nebula/config"
	"github.com/slackhq/nebula/overlay"
	"gopkg.in/yaml.v3"
)

// These tests run real Nebula UDP sockets, Noise handshakes, certificates and
// firewall/relay code. Only the OS TUN device is replaced by upstream UserDevice.
// They require no administrator privileges and do not test public NAT or Wintun.
type testPeer struct {
	e       *Engine
	c       Config
	in      *io.PipeWriter
	out     chan []byte
	address string
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func testLease(t *testing.T, ca *pki.Authority, id, ip, room string, node *model.Node) Config {
	t.Helper()
	key, pub, err := pki.TunnelKey()
	must(t, err)
	group := "room:" + room
	if node != nil {
		group = "infrastructure"
	}
	leaf, err := ca.Sign(id, netip.MustParsePrefix(ip+"/16"), []string{group}, pub, time.Now().Add(9*time.Minute))
	must(t, err)
	pem, err := leaf.MarshalPEM()
	must(t, err)
	fp, err := leaf.Fingerprint()
	must(t, err)
	return Config{DeviceID: id, PrivateKey: key, DisableTUN: true, Lease: model.Lease{IP: ip, Network: model.DefaultPool, RoomID: room, Node: node, CA: ca.PEM, Certificate: string(pem), Fingerprint: fp, ExpiresAt: leaf.NotAfter()}}
}
func publicAddress(t *testing.T, host string) string {
	t.Helper()
	c, err := net.ListenPacket("udp4", host+":0")
	must(t, err)
	a := c.LocalAddr().String()
	must(t, c.Close())
	return a
}
func startTestPeer(t *testing.T, c Config, address string, tweak func(map[string]any)) *testPeer {
	t.Helper()
	host, port, err := net.SplitHostPort(address)
	must(t, err)
	c.ListenHost = host
	c.ListenPort, err = strconv.Atoi(port)
	must(t, err)
	raw, err := Render(c)
	must(t, err)
	var v map[string]any
	must(t, yaml.Unmarshal([]byte(raw), &v))
	v["logging"] = map[string]any{"level": "error"}
	if tweak != nil {
		tweak(v)
	}
	b, err := yaml.Marshal(v)
	must(t, err)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.NewC(log)
	must(t, cfg.LoadString(string(b)))
	p := &testPeer{c: c, address: address, out: make(chan []byte, 128)}
	var reader *io.PipeReader
	factory := func(_ *config.C, _ *slog.Logger, networks []netip.Prefix, _ int) (overlay.Device, error) {
		d, err := overlay.NewUserDevice(networks)
		if err == nil {
			reader, p.in = d.(*overlay.UserDevice).Pipe()
			go func(reader *io.PipeReader) {
				buf := make([]byte, 65536)
				for {
					n, err := reader.Read(buf)
					if err != nil {
						return
					}
					select {
					case p.out <- bytes.Clone(buf[:n]):
					default:
					}
				}
			}(reader)
		}
		return d, err
	}
	ctrl, err := nebula.Main(cfg, false, model.NebulaVersion, log, factory)
	must(t, err)
	p.e = &Engine{control: ctrl, config: cfg, applied: c, raw: raw, log: log, deviceFactory: factory}
	must(t, ctrl.Start())
	t.Cleanup(p.e.Stop)
	return p
}
func (p *testPeer) packet(t *testing.T, dst string, port uint16, payload string) {
	t.Helper()
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP(p.c.Lease.IP), DstIP: net.ParseIP(dst)}
	udp := &layers.UDP{SrcPort: 45000, DstPort: layers.UDPPort(port)}
	must(t, udp.SetNetworkLayerForChecksum(ip))
	b := gopacket.NewSerializeBuffer()
	must(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ip, udp, gopacket.Payload(payload)))
	_, err := p.in.Write(b.Bytes())
	must(t, err)
}
func delivered(t *testing.T, from, to *testPeer, port uint16, payload string, want bool) {
	t.Helper()
	timeout := 5 * time.Second
	if !want {
		timeout = 500 * time.Millisecond
	}
	end := time.NewTimer(timeout)
	defer end.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	from.packet(t, to.c.Lease.IP, port, payload)
	for {
		select {
		case b := <-to.out:
			if bytes.Contains(b, []byte(payload)) {
				if !want {
					t.Fatal("unauthorized packet passed firewall")
				}
				return
			}
		case <-tick.C:
			from.packet(t, to.c.Lease.IP, port, payload)
		case <-end.C:
			if want {
				t.Fatalf("Nebula did not deliver %s; sender=%+v receiver=%+v", payload, from.e.control.ListHostmapHosts(false), to.e.control.ListHostmapHosts(false))
			}
			return
		}
	}
}
func TestNebulaIsolationReloadAndRevocation(t *testing.T) {
	ca, err := pki.Generate(netip.MustParsePrefix(model.DefaultPool))
	must(t, err)
	n := model.Node{ID: "lh", IP: "10.203.0.1", Address: publicAddress(t, "127.0.0.1"), Lighthouse: true, Relay: true}
	room := &model.Room{ID: "room-one", ExpiresAt: time.Now().Add(time.Hour)}
	s := model.Snapshot{Room: room, Nodes: []model.Node{n}, Members: []model.Member{{DeviceID: "alice", IP: "10.203.0.2"}, {DeviceID: "bob", IP: "10.203.0.3"}}, Game: &model.Game{Enabled: true, Network: model.GameNetwork{Version: 1}}}
	lc := testLease(t, ca, n.ID, n.IP, "", &n)
	lc.Snapshot = s
	startTestPeer(t, lc, n.Address, nil)
	a := testLease(t, ca, "alice", "10.203.0.2", room.ID, nil)
	a.Snapshot = s
	b := testLease(t, ca, "bob", "10.203.0.3", room.ID, nil)
	b.Snapshot = s
	alice := startTestPeer(t, a, publicAddress(t, "127.0.0.1"), nil)
	bob := startTestPeer(t, b, publicAddress(t, "127.0.0.1"), nil)
	a, b = alice.c, bob.c
	delivered(t, alice, bob, model.LANPort, "authorized", true)
	if got := alice.e.Peers(s.Members)[0].Mode; got != "direct" {
		t.Fatalf("path=%s, want direct", got)
	}
	observed, _ := alice.e.Network()
	found := false
	for _, peer := range observed.Peers {
		if peer.DeviceID == "bob" || peer.IP == bob.c.Lease.IP {
			found = peer.Mode == "direct" && peer.Remote == bob.address
		}
	}
	if !found {
		t.Fatal("telemetry did not expose the actual direct remote")
	}
	// A third device uses a valid CA certificate but belongs to another room.
	c := testLease(t, ca, "mallory", "10.203.0.4", "room-two", nil)
	c.Snapshot = model.Snapshot{Game: s.Game, Room: &model.Room{ID: "room-two"}, Nodes: s.Nodes, Members: []model.Member{{DeviceID: "mallory", IP: c.Lease.IP}}}
	mal := startTestPeer(t, c, publicAddress(t, "127.0.0.1"), func(v map[string]any) {
		v["firewall"].(map[string]any)["outbound"] = []map[string]any{{"proto": "any", "port": "any", "host": "any"}}
	})
	delivered(t, mal, bob, model.LANPort, "cross-room", false)
	delivered(t, alice, bob, 7001, "closed-port", false)
	// Renewal changes the certificate without replacing the virtual device.
	fresh := testLease(t, ca, "bob", b.Lease.IP, room.ID, nil)
	fresh.Snapshot = b.Snapshot
	fresh.ListenHost = b.ListenHost
	fresh.ListenPort = b.ListenPort
	generation := bob.e.Generation()
	must(t, bob.e.Apply(fresh))
	if bob.e.Generation() != generation {
		t.Fatal("certificate renewal restarted device")
	}
	b = fresh
	delivered(t, alice, bob, model.LANPort, "renewed-cert", true)
	b.Snapshot.Blocklist = []string{a.Lease.Fingerprint}
	b.Snapshot.Members = []model.Member{s.Members[1]}
	must(t, bob.e.Apply(b))
	if bob.e.control.GetHostInfoByVpnAddr(netip.MustParseAddr(a.Lease.IP), false) != nil {
		t.Fatal("revoked tunnel retained")
	}
	delivered(t, alice, bob, model.LANPort, "old-cert-reconnect", false)
	bob.e.Expire(b.Lease.ExpiresAt.Add(time.Second))
	if bob.e.Running() {
		t.Fatal("expired local credential still runs")
	}
}

func TestNebulaNativeRelay(t *testing.T) {
	ca, err := pki.Generate(netip.MustParsePrefix(model.DefaultPool))
	must(t, err)
	n := model.Node{ID: "relay", IP: "10.203.0.1", Address: publicAddress(t, "127.0.0.10"), Lighthouse: true, Relay: true}
	n2 := model.Node{ID: "relay-two", IP: "10.203.0.4", Address: publicAddress(t, "127.0.0.11"), Lighthouse: true, Relay: true}
	s := model.Snapshot{Room: &model.Room{ID: "room"}, Nodes: []model.Node{n, n2}, Members: []model.Member{{DeviceID: "a", IP: "10.203.0.2"}, {DeviceID: "b", IP: "10.203.0.3"}}, Game: &model.Game{Enabled: true, Network: model.GameNetwork{Version: 1}}}
	lc := testLease(t, ca, n.ID, n.IP, "", &n)
	lc.Snapshot = s
	first := startTestPeer(t, lc, n.Address, nil)
	lc2 := testLease(t, ca, n2.ID, n2.IP, "", &n2)
	lc2.Snapshot = s
	second := startTestPeer(t, lc2, n2.Address, nil)
	blockDirect := func(v map[string]any) {
		lh := v["lighthouse"].(map[string]any)
		lh["local_allow_list"] = map[string]bool{"0.0.0.0/0": false}
		lh["remote_allow_list"] = map[string]bool{"0.0.0.0/0": true, "127.0.0.2/32": false, "127.0.0.3/32": false}
	}
	var peers []*testPeer
	for i, m := range s.Members {
		c := testLease(t, ca, m.DeviceID, m.IP, s.Room.ID, nil)
		c.Snapshot = s
		peers = append(peers, startTestPeer(t, c, publicAddress(t, fmt.Sprintf("127.0.0.%d", i+2)), blockDirect))
	}
	delivered(t, peers[0], peers[1], model.LANPort, "native-relay", true)
	if got := peers[0].e.Peers(s.Members)[0].Mode; got != "relay" {
		t.Fatalf("path=%s, want relay", got)
	}
	observed, _ := peers[0].e.Network()
	found := false
	for _, peer := range observed.Peers {
		if peer.DeviceID == "b" {
			found = peer.Mode == "relay" && peer.Remote == "" && len(peer.RelayIPs) > 0
		}
	}
	if !found {
		t.Fatal("telemetry substituted a relay candidate for the actual path")
	}
	h := peers[0].e.control.GetHostInfoByVpnAddr(netip.MustParseAddr(peers[1].c.Lease.IP), false)
	survivor := n2
	if h.CurrentRelaysToMe[0].String() == n.IP {
		first.e.Stop()
	} else {
		second.e.Stop()
		survivor = n
	}
	for _, p := range peers {
		p.c.Snapshot.Nodes = []model.Node{survivor}
		p.c.RelayIPs = []string{survivor.IP}
		must(t, p.e.Apply(p.c))
		// Restore the test-only direct path prohibition after the product config
		// update; production relies on real NAT/firewall reachability instead.
		raw, err := Render(p.c)
		must(t, err)
		var v map[string]any
		must(t, yaml.Unmarshal([]byte(raw), &v))
		blockDirect(v)
		encoded, err := yaml.Marshal(v)
		must(t, err)
		must(t, p.e.config.ReloadConfigString(string(encoded)))
	}
	delivered(t, peers[0], peers[1], model.LANPort, "replacement-relay", true)
	if got := peers[0].e.Peers(s.Members)[0].Mode; got != "relay" {
		t.Fatalf("failover path=%s, want relay", got)
	}
}

func TestNebulaP2PWithoutRelay(t *testing.T) {
	ca, err := pki.Generate(netip.MustParsePrefix(model.DefaultPool))
	must(t, err)
	n := model.Node{ID: "lighthouse", IP: "10.203.0.1", Address: publicAddress(t, "127.0.0.1"), Lighthouse: true}
	s := model.Snapshot{Room: &model.Room{ID: "p2p-room"}, Nodes: []model.Node{n}, Members: []model.Member{{DeviceID: "a", IP: "10.203.0.2"}, {DeviceID: "b", IP: "10.203.0.3"}}, Game: &model.Game{Enabled: true, Network: model.GameNetwork{Version: 1}}}
	lc := testLease(t, ca, n.ID, n.IP, "", &n)
	lc.Snapshot = s
	startTestPeer(t, lc, n.Address, nil)
	var peers []*testPeer
	for _, m := range s.Members {
		c := testLease(t, ca, m.DeviceID, m.IP, s.Room.ID, nil)
		c.Snapshot, c.RelayIPs = s, []string{}
		peers = append(peers, startTestPeer(t, c, publicAddress(t, "127.0.0.1"), func(v map[string]any) {
			v["relay"].(map[string]any)["use_relays"] = false
		}))
	}
	for i, from := range peers {
		to := peers[1-i]
		delivered(t, from, to, model.LANPort, "p2p-without-relay", true)
		h := from.e.control.GetHostInfoByVpnAddr(netip.MustParseAddr(to.c.Lease.IP), false)
		if h == nil || h.CurrentRemote.String() != to.address || len(h.CurrentRelaysToMe) != 0 || from.e.Peers(s.Members)[0].Mode != "direct" {
			t.Fatal("P2P did not establish the actual direct peer endpoint")
		}
		delivered(t, from, to, 7001, "p2p-closed-port", false)
	}
}
