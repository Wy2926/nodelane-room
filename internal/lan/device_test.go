package lan

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/nodelane/nodelane-room/internal/model"
)

type memoryTAP struct {
	in, out chan []byte
	closed  chan struct{}
	once    sync.Once
}

func newMemoryTAP() *memoryTAP {
	return &memoryTAP{in: make(chan []byte, 128), out: make(chan []byte, 128), closed: make(chan struct{})}
}
func (t *memoryTAP) Read(b []byte) (int, error) {
	select {
	case <-t.closed:
		return 0, io.EOF
	case p := <-t.in:
		return copy(b, p), nil
	}
}
func (t *memoryTAP) Write(b []byte) (int, error) {
	select {
	case <-t.closed:
		return 0, io.ErrClosedPipe
	case t.out <- bytes.Clone(b):
		return len(b), nil
	}
}
func (t *memoryTAP) Close() error { t.once.Do(func() { close(t.closed) }); return nil }

func fixture(t *testing.T) (*Device, *Device, *memoryTAP, *memoryTAP, model.Snapshot) {
	t.Helper()
	now := time.Now()
	s := model.Snapshot{Room: &model.Room{ID: "room", ExpiresAt: now.Add(time.Hour)}, Game: &model.Game{ID: "game", Enabled: true, Revision: 1, Network: model.GameNetwork{Version: 1, Broadcast: true, Multicast: true}, Ports: []model.GamePort{{Protocol: "udp", Port: 7000, PortEnd: 7099}, {Protocol: "tcp", Port: 7000, PortEnd: 7099}}}, Members: []model.Member{{DeviceID: "alice", IP: "10.203.0.2", MAC: "02:01:02:03:04:02", LastSeen: now}, {DeviceID: "bob", IP: "10.203.0.3", MAC: "02:01:02:03:04:03", LastSeen: now}, {DeviceID: "carol", IP: "10.203.0.4", MAC: "02:01:02:03:04:04", LastSeen: now}}}
	newDevice := func(index int) (*Device, *memoryTAP) {
		m := s.Members[index]
		tap := newMemoryTAP()
		mac, _ := net.ParseMAC(m.MAC)
		d, err := New(tap, mac, netip.MustParsePrefix(m.IP+"/16"), model.Lease{RoomID: "room", ExpiresAt: now.Add(time.Minute)}, s)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { d.Close() })
		return d, tap
	}
	a, at := newDevice(0)
	b, bt := newDevice(1)
	return a, b, at, bt, s
}

func gameUDP(t *testing.T, from, to member, port uint16, payload []byte, ipv6 bool) []byte {
	t.Helper()
	if !ipv6 {
		ip := udpPacket(from.ip, to.ip, port, payload)
		binary.BigEndian.PutUint16(ip[20:22], 45000)
		return ethernet(from.mac, to.mac, 0x0800, ip)
	}
	ip := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolUDP, SrcIP: net.IP(model.LANIPv6(from.ip).AsSlice()), DstIP: net.IP(model.LANIPv6(to.ip).AsSlice())}
	u := &layers.UDP{SrcPort: 45000, DstPort: layers.UDPPort(port)}
	if err := u.SetNetworkLayerForChecksum(ip); err != nil {
		t.Fatal(err)
	}
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ip, u, gopacket.Payload(payload)); err != nil {
		t.Fatal(err)
	}
	return ethernet(from.mac, to.mac, 0x86dd, buf.Bytes())
}

func wire(d *Device, f []byte, to member) [][]byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue(f, to)
	out := [][]byte{}
	for _, p := range d.pending {
		out = append(out, p.packet)
	}
	d.pending = nil
	return out
}
func injected(t *testing.T, tap *memoryTAP, want []byte) {
	t.Helper()
	select {
	case got := <-tap.out:
		if !bytes.Equal(got, want) {
			t.Fatal("Ethernet frame changed")
		}
	case <-time.After(time.Second):
		t.Fatal("frame not injected")
	}
}
func noInjection(t *testing.T, tap *memoryTAP) {
	t.Helper()
	select {
	case <-tap.out:
		t.Fatal("unauthorized frame injected")
	default:
	}
}

func TestReplayWindowDoesNotThrottleTrafficAndPolicyRejectsQueuedFrames(t *testing.T) {
	a, b, _, tap, snapshot := fixture(t)
	f := gameUDP(t, a.members[a.ip], a.members[b.ip], 7000, []byte("payload"), false)
	var first []byte
	for i := 0; i < 10000; i++ {
		p := wire(a, f, a.members[b.ip])[0]
		if i == 0 {
			first = bytes.Clone(p)
		}
		if _, err := b.Write(p); err != nil {
			t.Fatal(err)
		}
		injected(t, tap, f)
	}
	if _, err := b.Write(first); err != nil {
		t.Fatal(err)
	}
	noInjection(t, tap)
	generation := b.generation
	snapshot.Members = snapshot.Members[1:]
	if err := b.Update(model.Lease{RoomID: "room", ExpiresAt: time.Now().Add(time.Minute)}, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := b.deliver(f, generation); err != nil {
		t.Fatal(err)
	}
	noInjection(t, tap)
}

func TestEthernetMTUAndOutOfOrderReassembly(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		t.Run(map[bool]string{false: "IPv4", true: "IPv6"}[v6], func(t *testing.T) {
			a, b, _, tap, _ := fixture(t)
			n := 1472
			if v6 {
				n = 1452
			}
			f := gameUDP(t, a.members[a.ip], a.members[b.ip], 7000, bytes.Repeat([]byte{0x52}, n), v6)
			if len(f) != 1514 {
				t.Fatal(len(f))
			}
			packets := wire(a, f, a.members[b.ip])
			if len(packets) != 2 {
				t.Fatal(len(packets))
			}
			for i := len(packets) - 1; i >= 0; i-- {
				if len(packets[i]) > 1104 {
					t.Fatal("carrier exceeds reviewed MTU")
				}
				if _, err := b.Write(packets[i]); err != nil {
					t.Fatal(err)
				}
				if i != 0 {
					noInjection(t, tap)
				}
			}
			injected(t, tap, f)
			for _, p := range packets {
				b.Write(p)
			}
			noInjection(t, tap)
		})
	}
}

func TestEthernetInnerAuthorizationAndUDPReplies(t *testing.T) {
	a, b, _, tap, s := fixture(t)
	alice, bob := a.members[a.ip], a.members[b.ip]
	f := gameUDP(t, alice, bob, 7000, []byte("query"), false)
	for _, p := range wire(a, f, bob) {
		b.Write(p)
	}
	injected(t, tap, f)
	reply := udpPacket(bob.ip, alice.ip, 45000, []byte("reply"))
	binary.BigEndian.PutUint16(reply[20:22], 7000)
	r := ethernet(bob.mac, alice.mac, 0x0800, reply)
	if len(b.permitted(r, bob, alice, time.Now())) != 1 {
		t.Fatal("authorized ephemeral reply rejected")
	}
	bad := gameUDP(t, alice, bob, 7100, []byte("closed"), false)
	for _, p := range wire(a, bad, bob) {
		b.Write(p)
	}
	noInjection(t, tap)
	spoof := bytes.Clone(f)
	copy(spoof[6:12], a.members[netip.MustParseAddr("10.203.0.4")].mac)
	for _, p := range wire(a, spoof, bob) {
		b.Write(p)
	}
	noInjection(t, tap)
	s.Game = &model.Game{ID: "game", Enabled: true, Revision: 2, Network: s.Game.Network, Ports: []model.GamePort{{Protocol: "udp", Port: 7001}}}
	if err := b.Update(model.Lease{RoomID: "room", ExpiresAt: time.Now().Add(time.Minute)}, s); err != nil {
		t.Fatal(err)
	}
	if len(b.permitted(r, bob, alice, time.Now())) != 0 {
		t.Fatal("reply conntrack survived policy narrowing")
	}
}

func TestBroadcastAndMulticastKeepAddresses(t *testing.T) {
	for _, dst := range []string{"255.255.255.255", "10.203.255.255", "239.10.20.30"} {
		t.Run(dst, func(t *testing.T) {
			a, b, _, tap, _ := fixture(t)
			alice, bob := a.members[a.ip], a.members[b.ip]
			ip := udpPacket(alice.ip, netip.MustParseAddr(dst), 7000, []byte("LAN discovery"))
			binary.BigEndian.PutUint16(ip[20:22], 45000)
			mac := net.HardwareAddr{255, 255, 255, 255, 255, 255}
			if dst[0:3] == "239" {
				mac = net.HardwareAddr{1, 0, 0x5e, 10, 20, 30}
			}
			f := ethernet(alice.mac, mac, 0x0800, ip)
			if len(a.permitted(f, alice, bob, time.Now())) != 1 {
				t.Fatal("discovery rejected")
			}
			for _, p := range wire(a, f, bob) {
				b.Write(p)
			}
			injected(t, tap, f)
			b.broadcast = false
			b.multicast = false
			for _, p := range wire(a, f, bob) {
				b.Write(p)
			}
			noInjection(t, tap)
		})
	}
}

func TestFragmentsCannotBypassPortsOrSurviveRevocation(t *testing.T) {
	a, b, _, tap, s := fixture(t)
	alice, bob := a.members[a.ip], a.members[b.ip]
	f := gameUDP(t, alice, bob, 7000, bytes.Repeat([]byte{7}, 1472), false)
	p := wire(a, f, bob)
	b.Write(p[0])
	noInjection(t, tap)
	s.Members = s.Members[1:]
	if err := b.Update(model.Lease{RoomID: "room", ExpiresAt: time.Now().Add(time.Minute)}, s); err != nil {
		t.Fatal(err)
	}
	b.Write(p[1])
	noInjection(t, tap)
	if len(b.frames) != 0 {
		t.Fatal("revoked reassembly cache retained")
	}
	a, b, _, tap, _ = fixture(t)
	alice, bob = a.members[a.ip], a.members[b.ip]
	for _, port := range []uint16{7100, 7000} {
		whole := udpPacket(alice.ip, bob.ip, port, bytes.Repeat([]byte{9}, 2500))
		parts := [][]byte{}
		for offset := 0; offset < len(whole)-20; offset += 1200 {
			end := min(offset+1200, len(whole)-20)
			ip := append(bytes.Clone(whole[:20]), whole[20+offset:20+end]...)
			binary.BigEndian.PutUint16(ip[2:4], uint16(len(ip)))
			binary.BigEndian.PutUint16(ip[4:6], port)
			frag := uint16(offset / 8)
			if end < len(whole)-20 {
				frag |= 0x2000
			}
			binary.BigEndian.PutUint16(ip[6:8], frag)
			ip[10], ip[11] = 0, 0
			binary.BigEndian.PutUint16(ip[10:12], checksum(ip[:20]))
			parts = append(parts, ethernet(alice.mac, bob.mac, 0x0800, ip))
		}
		for i := len(parts) - 1; i >= 0; i-- {
			for _, p := range wire(a, parts[i], bob) {
				b.Write(p)
			}
		}
		if port == 7100 {
			noInjection(t, tap)
		} else {
			for i := len(parts) - 1; i >= 0; i-- {
				injected(t, tap, parts[i])
			}
		}
	}
}

func TestNonIPAndUnknownCarrierFailClosed(t *testing.T) {
	a, b, _, tap, _ := fixture(t)
	alice, bob := a.members[a.ip], a.members[b.ip]
	f := ethernet(alice.mac, bob.mac, 0x8137, bytes.Repeat([]byte{1}, 64))
	for _, p := range wire(a, f, bob) {
		b.Write(p)
	}
	noInjection(t, tap)
	b.etherTypes[0x8137] = true
	for _, p := range wire(a, f, bob) {
		b.Write(p)
	}
	injected(t, tap, f)
	p := wire(a, f, bob)[0]
	p[28+4] = 99
	b.Write(p)
	noInjection(t, tap)
	p = wire(a, f, bob)[0]
	p[28+36] ^= 1
	b.Write(p)
	noInjection(t, tap)
	p = wire(a, f, bob)[0]
	binary.BigEndian.PutUint16(p[28+8:28+10], 65535)
	b.Write(p)
	noInjection(t, tap)
	b.etherTypes[0] = true
	snap := ethernet(alice.mac, bob.mac, 40, append([]byte{0xaa, 0xaa, 3, 0, 0, 0, 8, 0}, bytes.Repeat([]byte{0}, 32)...))
	if len(b.permitted(snap, alice, bob, time.Now())) != 0 {
		t.Fatal("LLC bypassed IP validation")
	}
}

func TestCarrierReadAndProbeDoNotUseGamePorts(t *testing.T) {
	a, b, tap, _, _ := fixture(t)
	alice, bob := a.members[a.ip], a.members[b.ip]
	out := make(chan []byte, 8)
	go func() {
		buf := make([]byte, 1300)
		for {
			n, err := a.Read(buf)
			if err != nil {
				return
			}
			out <- bytes.Clone(buf[:n])
		}
	}()
	tap.in <- gameUDP(t, alice, bob, 7000, []byte("game"), false)
	select {
	case p := <-out:
		ip, ok := parseIP(p)
		if !ok || binary.BigEndian.Uint16(ip.data[2:4]) != model.LANPort {
			t.Fatal("game escaped carrier")
		}
	case <-time.After(time.Second):
		t.Fatal("TAP read blocked")
	}
	conn := a.ProbeConn()
	if _, err := conn.WriteTo([]byte("probe"), &net.UDPAddr{IP: net.ParseIP(b.ip.String()), Port: model.ProbePort}); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-out:
		ip, ok := parseIP(p)
		if !ok || binary.BigEndian.Uint16(ip.data[2:4]) != model.ProbePort {
			t.Fatal("probe escaped native IP")
		}
		b.Write(p)
	case <-time.After(time.Second):
		t.Fatal("probe waited for a TAP packet")
	}
	buf := make([]byte, 64)
	n, _, err := b.ProbeConn().ReadFrom(buf)
	if err != nil || string(buf[:n]) != "probe" {
		t.Fatal("probe not delivered")
	}
}
