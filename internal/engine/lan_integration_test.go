package engine

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/nodelane/nodelane-room/internal/lan"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
)

type ethernetTestTAP struct {
	in, out chan []byte
	closed  chan struct{}
	once    sync.Once
}

func (t *ethernetTestTAP) Read(b []byte) (int, error) {
	select {
	case <-t.closed:
		return 0, io.EOF
	case p := <-t.in:
		return copy(b, p), nil
	}
}
func (t *ethernetTestTAP) Write(b []byte) (int, error) {
	select {
	case <-t.closed:
		return 0, io.ErrClosedPipe
	case t.out <- bytes.Clone(b):
		return len(b), nil
	}
}
func (t *ethernetTestTAP) Close() error { t.once.Do(func() { close(t.closed) }); return nil }

type ethernetTestPeer struct {
	e   *Engine
	c   Config
	tap *ethernetTestTAP
}

func startEthernetPeer(t *testing.T, c Config, mac string) *ethernetTestPeer {
	t.Helper()
	c.ListenHost = "127.0.0.1"
	p := &ethernetTestPeer{c: c, e: New(slog.New(slog.NewTextHandler(io.Discard, nil)))}
	p.e.tapFactory = func(name string, _ netip.Prefix) (*lan.TAP, error) {
		p.tap = &ethernetTestTAP{in: make(chan []byte, 128), out: make(chan []byte, 128), closed: make(chan struct{})}
		m, _ := net.ParseMAC(mac)
		return &lan.TAP{ReadWriteCloser: p.tap, MAC: m, Name: name, Activate: func() error { return nil }}, nil
	}
	must(t, p.e.Apply(c))
	t.Cleanup(p.e.Stop)
	return p
}

func ethernetUDP(t *testing.T, from, to model.Member, destination string, port uint16, data []byte) []byte {
	t.Helper()
	src, _ := net.ParseMAC(from.MAC)
	dst, _ := net.ParseMAC(to.MAC)
	if destination == "255.255.255.255" {
		dst = net.HardwareAddr{255, 255, 255, 255, 255, 255}
	}
	eth := &layers.Ethernet{SrcMAC: src, DstMAC: dst, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP(from.IP), DstIP: net.ParseIP(destination)}
	udp := &layers.UDP{SrcPort: 45000, DstPort: layers.UDPPort(port)}
	must(t, udp.SetNetworkLayerForChecksum(ip))
	b := gopacket.NewSerializeBuffer()
	must(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(data)))
	return b.Bytes()
}

func ethernetDelivered(t *testing.T, from, to *ethernetTestPeer, frame []byte, want bool) {
	t.Helper()
	timeout := 5 * time.Second
	if !want {
		timeout = 350 * time.Millisecond
	}
	end := time.NewTimer(timeout)
	defer end.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	send := func() {
		select {
		case from.tap.in <- bytes.Clone(frame):
		default:
		}
	}
	send()
	for {
		select {
		case got := <-to.tap.out:
			if bytes.Equal(got, frame) {
				if !want {
					t.Fatal("unauthorized Ethernet frame delivered")
				}
				return
			}
		case <-tick.C:
			send()
		case <-end.C:
			if want {
				t.Fatalf("Ethernet delivery failed: %+v", from.e.Peers(from.c.Snapshot.Members))
			}
			return
		}
	}
}

func TestNebulaEthernetBroadcastMTUAndRevocation(t *testing.T) {
	ca, err := pki.Generate(netip.MustParsePrefix(model.DefaultPool))
	must(t, err)
	n := model.Node{ID: "lh", IP: "10.203.0.1", Address: publicAddress(t, "127.0.0.1"), Lighthouse: true}
	now := time.Now()
	s := model.Snapshot{Room: &model.Room{ID: "ethernet", ExpiresAt: now.Add(time.Hour)}, Game: &model.Game{ID: "ethernet", Enabled: true, Revision: 1, Ports: []model.GamePort{{Protocol: "udp", Port: 1, PortEnd: 65535}}, Network: model.GameNetwork{Version: 1, Broadcast: true, Multicast: true}}, Nodes: []model.Node{n}, Members: []model.Member{{DeviceID: "alice", IP: "10.203.0.2", MAC: "02:00:00:00:00:02", LastSeen: now}, {DeviceID: "bob", IP: "10.203.0.3", MAC: "02:00:00:00:00:03", LastSeen: now}, {DeviceID: "carol", IP: "10.203.0.4", MAC: "02:00:00:00:00:04", LastSeen: now}}}
	lc := testLease(t, ca, n.ID, n.IP, "", &n)
	lc.Snapshot = s
	startTestPeer(t, lc, n.Address, nil)
	peers := []*ethernetTestPeer{}
	for _, m := range s.Members {
		c := testLease(t, ca, m.DeviceID, m.IP, s.Room.ID, nil)
		c.Snapshot = s
		peers = append(peers, startEthernetPeer(t, c, m.MAC))
	}
	a, b, c := peers[0], peers[1], peers[2]
	f := ethernetUDP(t, s.Members[0], s.Members[1], s.Members[1].IP, 65535, bytes.Repeat([]byte("m"), 1472))
	if len(f) != 1514 {
		t.Fatal(len(f))
	}
	ethernetDelivered(t, a, b, f, true)
	mode, _ := a.e.Path(b.c.Lease.IP)
	if mode != "direct" {
		t.Fatal(mode)
	}
	broadcast := ethernetUDP(t, s.Members[0], s.Members[1], "255.255.255.255", 7000, []byte("broadcast-source-preserved"))
	ethernetDelivered(t, a, b, broadcast, true)
	ethernetDelivered(t, a, c, broadcast, true)
	// Game UDP 4243 is encapsulated independently of the diagnostic socket.
	gameProbePort := ethernetUDP(t, s.Members[0], s.Members[1], s.Members[1].IP, model.ProbePort, []byte("game-port-4243"))
	ethernetDelivered(t, a, b, gameProbePort, true)
	// A range remains one rule and narrowing closes the old connection state.
	g := *s.Game
	g.Revision++
	g.Ports = []model.GamePort{{Protocol: "udp", Port: 7000}}
	b.c.Snapshot.Game = &g
	generation := b.e.Generation()
	must(t, b.e.Apply(b.c))
	if b.e.Generation() == generation {
		t.Fatal("LAN rule change did not Stop/Wait")
	}
	a.c.Snapshot.Game = &g
	must(t, a.e.Apply(a.c))
	ethernetDelivered(t, a, b, f, false)
	fresh := ethernetUDP(t, s.Members[0], s.Members[1], s.Members[1].IP, 7000, []byte("new-policy"))
	ethernetDelivered(t, a, b, fresh, true)
	b.c.Snapshot.Members = []model.Member{s.Members[1], s.Members[2]}
	b.c.Snapshot.Blocklist = []string{a.c.Lease.Fingerprint}
	must(t, b.e.Apply(b.c))
	ethernetDelivered(t, a, b, fresh, false)
	b.e.Expire(b.c.Lease.ExpiresAt.Add(time.Second))
	if b.e.Running() {
		t.Fatal("expired Ethernet device still running")
	}
}
