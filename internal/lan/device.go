package lan

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

// 1104-byte inner IPv4 packets leave room for Nebula encryption, native relay
// encapsulation and an IPv6 underlay within a 1280-byte path MTU.
const chunkSize = 1024
const frameHeader = 52

type frameKey struct {
	peer       netip.Addr
	epoch, seq uint64
}
type frameSet struct {
	expires time.Time
	data    []byte
	parts   byte
}

type sessionKey struct {
	peer  netip.Addr
	epoch uint64
}

// A sequence window bounds memory without imposing a frames-per-second limit.
type replayWindow struct {
	highest uint64
	seq     [4096]uint64
	expires time.Time
}
type pending struct {
	packet     []byte
	peer       member
	generation uint64
}

// Device adapts an Ethernet TAP to Nebula's IP device API. The TAP is the only
// game-facing interface. Encapsulated packets never enter the host IP stack.
type Device struct {
	tap                           io.ReadWriteCloser
	mac                           net.HardwareAddr
	ip                            netip.Addr
	network                       netip.Prefix
	mu                            sync.Mutex
	readMu                        sync.Mutex
	writeMu                       sync.Mutex
	closeOnce                     sync.Once
	closed                        chan struct{}
	members                       map[netip.Addr]member
	nodes                         map[netip.Addr]bool
	expires                       time.Time
	broadcastIP                   netip.Addr
	enabled, broadcast, multicast bool
	ports                         [2][65536]bool
	etherTypes                    map[uint16]bool
	flows                         map[flow]time.Time
	ipFragments                   map[fragmentKey]*fragmentSet
	frames                        map[frameKey]*frameSet
	seen                          map[sessionKey]*replayWindow
	flowSweep                     time.Time
	room                          [16]byte
	revision                      int64
	epoch, seq, generation        uint64
	pending                       []pending
	tapIn                         chan tapRead
	tapDone                       chan struct{}
	probe                         *probeConn
	probeIn                       chan probeDatagram
	probeOut                      chan []byte
}

func New(tap io.ReadWriteCloser, mac net.HardwareAddr, prefix netip.Prefix, lease model.Lease, s model.Snapshot) (*Device, error) {
	if _, err := model.ParseLANMAC(mac.String()); err != nil {
		return nil, err
	}
	if !prefix.Addr().Is4() {
		return nil, errors.New("LAN carrier requires a certificate IPv4 address")
	}
	d := &Device{tap: tap, mac: bytes.Clone(mac), ip: prefix.Addr(), network: prefix, closed: make(chan struct{}), flows: map[flow]time.Time{}, ipFragments: map[fragmentKey]*fragmentSet{}, frames: map[frameKey]*frameSet{}, seen: map[sessionKey]*replayWindow{}}
	var epoch [8]byte
	if _, err := rand.Read(epoch[:]); err != nil {
		return nil, err
	}
	d.epoch = binary.BigEndian.Uint64(epoch[:])
	if err := d.Update(lease, s); err != nil {
		return nil, err
	}
	d.tapIn = make(chan tapRead, 128)
	d.tapDone = make(chan struct{})
	d.probeIn = make(chan probeDatagram, 128)
	d.probeOut = make(chan []byte, 128)
	d.probe = &probeConn{device: d, done: make(chan struct{})}
	go d.readTAP()
	return d, nil
}

func (d *Device) MAC() string { return d.mac.String() }

func (d *Device) Status() model.LANStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return model.LANStatus{Version: model.LANVersion, MAC: d.mac.String(), IPv6: model.LANIPv6(d.ip).String(), MTU: 1500, Ready: d.enabled && d.active(d.members[d.ip], time.Now()) && bytes.Equal(d.members[d.ip].mac, d.mac)}
}

func (d *Device) Update(lease model.Lease, s model.Snapshot) error {
	// Serialize policy publication with TAP injection: after Update returns,
	// no frame accepted by an older generation can reach the host.
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	if s.Game == nil || s.Room == nil || s.Room.ID != lease.RoomID || s.Room.Closed {
		return errors.New("LAN room policy missing")
	}
	if err := model.ValidateLAN(*s.Game); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.generation++
	d.expires = time.Time{}
	expires := lease.ExpiresAt
	if s.Room.ExpiresAt.Before(expires) {
		expires = s.Room.ExpiresAt
	}
	hash := sha256.Sum256([]byte(lease.RoomID))
	copy(d.room[:], hash[:16])
	if d.revision != s.Game.Revision {
		clear(d.flows)
		clear(d.ipFragments)
		clear(d.frames)
		clear(d.seen)
	}
	d.revision = s.Game.Revision
	d.enabled = s.Game.Enabled
	d.broadcast = s.Game.Network.Broadcast
	d.multicast = s.Game.Network.Multicast
	d.ports = [2][65536]bool{}
	for _, p := range s.Game.Ports {
		idx := 0
		if p.Protocol == "udp" {
			idx = 1
		}
		for port := int(p.Port); port <= int(model.PortEnd(p)); port++ {
			d.ports[idx][port] = true
		}
	}
	d.etherTypes = map[uint16]bool{}
	for _, typ := range s.Game.Network.EthernetTypes {
		d.etherTypes[typ] = true
	}
	previous := d.members
	d.members = map[netip.Addr]member{}
	macs := map[string]bool{}
	ips := map[netip.Addr]bool{}
	for _, m := range s.Members {
		ip, err := netip.ParseAddr(m.IP)
		if err != nil || !ip.Is4() || !d.network.Contains(ip) || ips[ip] {
			return errors.New("invalid LAN member address")
		}
		ips[ip] = true
		if m.MAC == "" {
			continue
		}
		mac, err := model.ParseLANMAC(m.MAC)
		if err != nil || macs[mac.String()] {
			return errors.New("invalid or duplicate LAN member MAC")
		}
		macs[mac.String()] = true
		d.members[ip] = member{ip, mac, m.LastSeen.Add(45 * time.Second)}
	}
	changed := len(previous) != len(d.members)
	for ip, m := range d.members {
		if !bytes.Equal(previous[ip].mac, m.mac) {
			changed = true
		}
	}
	if changed {
		clear(d.flows)
		clear(d.ipFragments)
		clear(d.frames)
	}
	d.nodes = map[netip.Addr]bool{}
	for _, n := range s.Nodes {
		ip, err := netip.ParseAddr(n.IP)
		if err == nil && ip.Is4() {
			d.nodes[ip] = true
		}
	}
	b := d.network.Masked().Addr().As4()
	v := binary.BigEndian.Uint32(b[:]) | uint32((uint64(1)<<(32-d.network.Bits()))-1)
	binary.BigEndian.PutUint32(b[:], v)
	d.broadcastIP = netip.AddrFrom4(b)
	d.expires = expires
	return nil
}

func (d *Device) Close() error {
	var err error
	d.closeOnce.Do(func() { close(d.closed); err = d.tap.Close(); <-d.tapDone })
	return err
}

func (d *Device) Read(b []byte) (int, error) {
	d.readMu.Lock()
	defer d.readMu.Unlock()
	for {
		select {
		case <-d.closed:
			return 0, io.EOF
		default:
		}
		d.mu.Lock()
		for len(d.pending) > 0 {
			p := d.pending[0]
			d.pending = d.pending[1:]
			if p.generation != d.generation || !d.active(p.peer, time.Now()) || !d.active(d.members[d.ip], time.Now()) {
				continue
			}
			if len(b) < len(p.packet) {
				d.mu.Unlock()
				return 0, io.ErrShortBuffer
			}
			n := copy(b, p.packet)
			d.mu.Unlock()
			return n, nil
		}
		d.mu.Unlock()
		var input tapRead
		select {
		case <-d.closed:
			return 0, io.EOF
		case raw := <-d.probeOut:
			p, ok := parseIP(raw)
			d.mu.Lock()
			allowed := ok && d.active(d.members[d.ip], time.Now()) && (d.nodes[p.dst] || d.active(d.members[p.dst], time.Now()))
			d.mu.Unlock()
			if !allowed {
				continue
			}
			if len(b) < len(raw) {
				return 0, io.ErrShortBuffer
			}
			return copy(b, raw), nil
		case input = <-d.tapIn:
		}
		if input.err != nil {
			return 0, input.err
		}
		if len(input.data) < 14 || len(input.data) > maxFrame {
			continue
		}
		frame := input.data
		d.mu.Lock()
		now := time.Now()
		own := d.members[d.ip]
		if !d.active(own, now) || !bytes.Equal(own.mac, d.mac) {
			d.mu.Unlock()
			continue
		}
		// Infrastructure diagnostics keep using native IP packets. They are not
		// exposed through the Ethernet fanout to infrastructure certificates.
		if raw := d.diagnostic(frame, now); raw != nil {
			d.mu.Unlock()
			if len(b) < len(raw) {
				return 0, io.ErrShortBuffer
			}
			return copy(b, raw), nil
		}
		if reply := d.nodeARP(frame); reply != nil {
			generation := d.generation
			d.mu.Unlock()
			if err := d.deliver(reply, generation); err != nil {
				return 0, err
			}
			continue
		}
		for ip, peer := range d.members {
			if ip == d.ip {
				continue
			}
			for _, f := range d.permitted(frame, own, peer, now) {
				d.queue(f, peer)
			}
		}
		d.mu.Unlock()
	}
}

func (d *Device) queue(frame []byte, peer member) {
	if len(d.pending)+2 > 4096 {
		return
	}
	d.seq++
	for offset := 0; offset < len(frame); offset += chunkSize {
		n := min(chunkSize, len(frame)-offset)
		h := make([]byte, frameHeader+n)
		copy(h, "NLAN")
		h[4] = model.LANVersion
		binary.BigEndian.PutUint16(h[6:8], uint16(offset))
		binary.BigEndian.PutUint16(h[8:10], uint16(len(frame)))
		binary.BigEndian.PutUint16(h[10:12], uint16(n))
		binary.BigEndian.PutUint64(h[12:20], d.epoch)
		binary.BigEndian.PutUint64(h[20:28], d.seq)
		binary.BigEndian.PutUint64(h[28:36], uint64(d.revision))
		copy(h[36:52], d.room[:])
		copy(h[52:], frame[offset:offset+n])
		d.pending = append(d.pending, pending{udpPacket(d.ip, peer.ip, model.LANPort, h), peer, d.generation})
	}
}

func (d *Device) Write(b []byte) (int, error) {
	p, ok := parseIP(b)
	if !ok || !p.src.Is4() || p.dst != d.ip || p.fragmented {
		return len(b), nil
	}
	d.mu.Lock()
	now := time.Now()
	own := d.members[d.ip]
	peer := d.members[p.src]
	if !d.active(own, now) {
		d.mu.Unlock()
		return len(b), nil
	}
	if (d.nodes[p.src] || d.active(peer, now)) && p.proto == 17 && len(p.data) >= 8 && binary.BigEndian.Uint16(p.data[:2]) == model.ProbePort && binary.BigEndian.Uint16(p.data[2:4]) == model.ProbePort && int(binary.BigEndian.Uint16(p.data[4:6])) == len(p.data) {
		if len(p.data) <= 1032 {
			select {
			case d.probeIn <- probeDatagram{bytes.Clone(p.data[8:]), p.src}:
			default:
			}
		}
		d.mu.Unlock()
		return len(b), nil
	}
	if raw := d.inboundDiagnostic(p, b, now); raw != nil {
		generation := d.generation
		d.mu.Unlock()
		return len(b), d.deliver(raw, generation)
	}
	if !d.active(peer, now) || p.proto != 17 || len(p.data) < 8+frameHeader || binary.BigEndian.Uint16(p.data[:2]) != model.LANPort || binary.BigEndian.Uint16(p.data[2:4]) != model.LANPort || int(binary.BigEndian.Uint16(p.data[4:6])) != len(p.data) {
		d.mu.Unlock()
		return len(b), nil
	}
	h := p.data[8:]
	if string(h[:4]) != "NLAN" || h[4] != model.LANVersion || h[5] != 0 || int64(binary.BigEndian.Uint64(h[28:36])) != d.revision || !bytes.Equal(h[36:52], d.room[:]) {
		d.mu.Unlock()
		return len(b), nil
	}
	offset, total, n := int(binary.BigEndian.Uint16(h[6:8])), int(binary.BigEndian.Uint16(h[8:10])), int(binary.BigEndian.Uint16(h[10:12]))
	if total < 14 || total > maxFrame || offset%chunkSize != 0 || offset >= total || n != min(chunkSize, total-offset) || len(h) != frameHeader+n {
		d.mu.Unlock()
		return len(b), nil
	}
	key := frameKey{p.src, binary.BigEndian.Uint64(h[12:20]), binary.BigEndian.Uint64(h[20:28])}
	for k, s := range d.frames {
		if !now.Before(s.expires) {
			delete(d.frames, k)
		}
	}
	for k, window := range d.seen {
		if !now.Before(window.expires) {
			delete(d.seen, k)
		}
	}
	sk := sessionKey{key.peer, key.epoch}
	window := d.seen[sk]
	if key.seq == 0 || window != nil && (window.seq[key.seq%4096] == key.seq || key.seq < window.highest && window.highest-key.seq >= 4096) {
		d.mu.Unlock()
		return len(b), nil
	}
	s := d.frames[key]
	if s == nil {
		if len(d.frames) >= 128 || window == nil && len(d.seen) >= 128 {
			d.mu.Unlock()
			return len(b), nil
		}
		s = &frameSet{expires: now.Add(2 * time.Second), data: make([]byte, total)}
		d.frames[key] = s
	}
	bit := byte(1 << (offset / chunkSize))
	if len(s.data) != total || s.parts&bit != 0 {
		d.mu.Unlock()
		return len(b), nil
	}
	copy(s.data[offset:], h[frameHeader:])
	s.parts |= bit
	if s.parts != byte((1<<((total+chunkSize-1)/chunkSize))-1) {
		d.mu.Unlock()
		return len(b), nil
	}
	delete(d.frames, key)
	if window == nil {
		window = &replayWindow{}
		d.seen[sk] = window
	}
	window.seq[key.seq%4096] = key.seq
	window.highest = max(window.highest, key.seq)
	window.expires = now.Add(2 * time.Minute)
	frames := d.permitted(s.data, peer, own, now)
	generation := d.generation
	d.mu.Unlock()
	for _, frame := range frames {
		if err := d.deliver(frame, generation); err != nil {
			return 0, err
		}
	}
	return len(b), nil
}

func (d *Device) deliver(frame []byte, generation uint64) error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	d.mu.Lock()
	allowed := generation == d.generation && d.active(d.members[d.ip], time.Now())
	d.mu.Unlock()
	if !allowed {
		return nil
	}
	select {
	case <-d.closed:
		return io.ErrClosedPipe
	default:
	}
	n, err := d.tap.Write(frame)
	if err == nil && n != len(frame) {
		return io.ErrShortWrite
	}
	return err
}

func (d *Device) diagnostic(frame []byte, now time.Time) []byte {
	if binary.BigEndian.Uint16(frame[12:14]) != 0x0800 {
		return nil
	}
	p, ok := parseIP(frame[14:])
	if !ok || p.src != d.ip || p.fragmented || !bytes.Equal(frame[6:12], d.mac) {
		return nil
	}
	if !d.nodes[p.dst] {
		return nil
	}
	if p.proto == 1 && validICMP(p.data) {
		return bytes.Clone(frame[14:])
	}
	return nil
}

func (d *Device) inboundDiagnostic(p packet, b []byte, now time.Time) []byte {
	if !d.nodes[p.src] && !d.active(d.members[p.src], now) {
		return nil
	}
	if p.proto != 1 || !validICMP(p.data) {
		return nil
	}
	mac := d.members[p.src].mac
	if len(mac) != 6 {
		v := p.src.As4()
		mac = net.HardwareAddr{2, 0x4e, v[0], v[1], v[2], v[3]}
	}
	return ethernet(mac, d.mac, 0x0800, b)
}

func (d *Device) nodeARP(frame []byte) []byte {
	if len(frame) < 42 || binary.BigEndian.Uint16(frame[12:14]) != 0x0806 {
		return nil
	}
	b := frame[14:]
	if binary.BigEndian.Uint16(b[:2]) != 1 || binary.BigEndian.Uint16(b[2:4]) != 0x0800 || b[4] != 6 || b[5] != 4 || binary.BigEndian.Uint16(b[6:8]) != 1 || !bytes.Equal(b[8:14], d.mac) || netip.AddrFrom4([4]byte(b[14:18])) != d.ip {
		return nil
	}
	ip := netip.AddrFrom4([4]byte(b[24:28]))
	if !d.nodes[ip] {
		return nil
	}
	v := ip.As4()
	mac := net.HardwareAddr{2, 0x4e, v[0], v[1], v[2], v[3]}
	reply := bytes.Clone(b[:28])
	binary.BigEndian.PutUint16(reply[6:8], 2)
	copy(reply[18:24], d.mac)
	copy(reply[24:28], d.ip.AsSlice())
	copy(reply[8:14], mac)
	copy(reply[14:18], ip.AsSlice())
	return ethernet(mac, d.mac, 0x0806, reply)
}
