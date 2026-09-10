package game

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
	"golang.org/x/net/ipv4"
)

const multicast = "224.0.2.60:4445"

func ParseAnnouncement(b []byte) (string, uint16, error) {
	if len(b) > 1024 {
		return "", 0, errors.New("announcement too large")
	}
	s := string(b)
	if !strings.HasPrefix(s, "[MOTD]") || !strings.HasSuffix(s, "[/AD]") || strings.Count(s, "[/MOTD][AD]") != 1 {
		return "", 0, errors.New("invalid LAN announcement")
	}
	parts := strings.SplitN(strings.TrimSuffix(strings.TrimPrefix(s, "[MOTD]"), "[/AD]"), "[/MOTD][AD]", 2)
	if len(parts) != 2 || (parts[0] != "" && !model.ValidLabel(parts[0], 512)) || strings.Contains(parts[0], "[/MOTD]") || strings.Contains(parts[0], "[AD]") {
		return "", 0, errors.New("invalid MOTD")
	}
	port, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil || port == 0 {
		return "", 0, errors.New("invalid game port")
	}
	return parts[0], uint16(port), nil
}
func Announcement(motd string, port uint16) []byte {
	return []byte(fmt.Sprintf("[MOTD]%s[/MOTD][AD]%d[/AD]", motd, port))
}
func LocalAddresses() map[string]bool {
	out := map[string]bool{"127.0.0.1": true, "::1": true}
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ip, _, err := net.ParseCIDR(a.String())
			if err == nil {
				out[ip.String()] = true
			}
		}
	}
	return out
}

type Observation struct {
	Protocol string
	Port     uint16
	MOTD     string
	Seen     time.Time
}
type remote struct {
	proxy    *Proxy
	endpoint model.Endpoint
	ip       string
	seen     time.Time
}
type Minecraft struct {
	mu            sync.Mutex
	socket        *net.UDPConn
	packet        *ipv4.PacketConn
	emitter       *net.UDPConn
	emitterPacket *ipv4.PacketConn
	localIP       string
	emitterPort   int
	local         map[uint16]Observation
	remotes       map[string]*remote
	snapshot      model.Snapshot
	device        string
	closed        bool
	events        chan DiscoveryEvent
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

func StartMinecraft(device string) (*Minecraft, error) {
	group, err := net.ResolveUDPAddr("udp4", multicast)
	if err != nil {
		return nil, err
	}
	socket, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		return nil, fmt.Errorf("Minecraft discovery port: %w", err)
	}
	packet := ipv4.NewPacketConn(socket)
	_ = packet.SetMulticastLoopback(true)
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp != 0 && i.Flags&net.FlagMulticast != 0 {
			_ = packet.JoinGroup(&i, group)
		}
	}
	route, err := net.DialUDP("udp4", nil, group)
	if err != nil {
		socket.Close()
		return nil, err
	}
	ip := route.LocalAddr().(*net.UDPAddr).IP
	route.Close()
	emitter, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip})
	if err != nil {
		socket.Close()
		return nil, err
	}
	ep := ipv4.NewPacketConn(emitter)
	if err = ep.SetMulticastTTL(0); err != nil {
		socket.Close()
		emitter.Close()
		return nil, err
	}
	_ = ep.SetMulticastLoopback(true)
	for _, i := range ifs {
		addrs, _ := i.Addrs()
		for _, addr := range addrs {
			a, _, _ := net.ParseCIDR(addr.String())
			if a != nil && a.Equal(ip) {
				_ = ep.SetMulticastInterface(&i)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Minecraft{socket: socket, packet: packet, emitter: emitter, emitterPacket: ep, localIP: ip.String(), emitterPort: emitter.LocalAddr().(*net.UDPAddr).Port, local: map[uint16]Observation{}, remotes: map[string]*remote{}, device: device, cancel: cancel, events: make(chan DiscoveryEvent, 128)}
	m.wg.Add(2)
	go m.collect(ctx)
	go m.publish(ctx)
	return m, nil
}
func (m *Minecraft) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.mu.Unlock()
	m.cancel()
	m.socket.Close()
	m.emitter.Close()
	m.wg.Wait()
	m.mu.Lock()
	for id, r := range m.remotes {
		r.proxy.Close()
		delete(m.remotes, id)
	}
	close(m.events)
	m.mu.Unlock()
}
func (m *Minecraft) Update(s model.Snapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.snapshot = s
	allowed := map[string]bool{}
	for _, e := range s.Endpoints {
		allowed[e.ID] = e.ExpiresAt.After(time.Now())
	}
	for id, r := range m.remotes {
		if !allowed[id] {
			m.emit(DiscoveryEvent{Kind: "removed", EndpointID: id, DeviceID: r.endpoint.DeviceID, Protocol: "tcp", Port: r.proxy.Port()})
			r.proxy.Close()
			delete(m.remotes, id)
		}
	}
}
func (m *Minecraft) Observations() []Observation {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Observation{}
	for port, o := range m.local {
		if time.Since(o.Seen) > 10*time.Second {
			delete(m.local, port)
			continue
		}
		out = append(out, o)
	}
	return out
}
func (m *Minecraft) collect(ctx context.Context) {
	defer m.wg.Done()
	b := make([]byte, 1025)
	var second int64
	var count int
	for {
		n, from, err := m.socket.ReadFromUDP(b)
		if err != nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
		if now := time.Now().Unix(); now != second {
			second = now
			count = 0
		}
		count++
		if count > 64 {
			continue
		}
		if n > 1024 || !LocalAddresses()[from.IP.String()] || from.Port == m.emitterPort {
			continue
		}
		motd, port, err := ParseAnnouncement(b[:n])
		if err != nil || port == model.ProbePort {
			continue
		}
		m.mu.Lock()
		generated := false
		for _, r := range m.remotes {
			if r.proxy.Port() == port {
				generated = true
			}
		}
		old, known := m.local[port]
		full := len(m.local) >= 32 && !known
		m.mu.Unlock()
		if generated || full {
			continue
		}
		if !known || time.Since(old.Seen) > 10*time.Second {
			conn, e := net.DialTimeout("tcp4", net.JoinHostPort(from.IP.String(), strconv.Itoa(int(port))), 250*time.Millisecond)
			if e != nil {
				continue
			}
			conn.Close()
		}
		m.mu.Lock()
		if !known || old.MOTD != motd {
			m.emit(DiscoveryEvent{Kind: "local", DeviceID: m.device, Protocol: "tcp", Port: port, MOTD: motd})
		}
		m.local[port] = Observation{Protocol: "tcp", Port: port, MOTD: motd, Seen: time.Now()}
		m.mu.Unlock()
	}
}

// Notice accepts only an endpoint independently authorized in the control snapshot.
func (m *Minecraft) Notice(ip, room, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.snapshot.Room == nil || m.snapshot.Room.ID != room || m.snapshot.Room.Closed {
		return
	}
	owner := ""
	for _, v := range m.snapshot.Members {
		if v.IP == ip {
			owner = v.DeviceID
		}
	}
	if owner == "" || owner == m.device {
		return
	}
	var endpoint *model.Endpoint
	for _, e := range m.snapshot.Endpoints {
		if e.ID == id && e.DeviceID == owner && e.Protocol == "tcp" && e.ExpiresAt.After(time.Now()) {
			v := e
			endpoint = &v
			break
		}
	}
	if endpoint == nil {
		return
	}
	if v := m.remotes[id]; v != nil {
		v.seen = time.Now()
		v.endpoint = *endpoint
		return
	}
	if len(m.remotes) >= 32 {
		return
	}
	proxy, err := NewProxy(m.localIP, net.JoinHostPort(ip, strconv.Itoa(int(endpoint.Port))))
	if err != nil {
		return
	}
	m.remotes[id] = &remote{proxy: proxy, endpoint: *endpoint, ip: ip, seen: time.Now()}
	m.emit(DiscoveryEvent{Kind: "remote", EndpointID: id, DeviceID: owner, Protocol: "tcp", Port: proxy.Port(), MOTD: endpoint.MOTD})
}
func (m *Minecraft) publish(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	group, _ := net.ResolveUDPAddr("udp4", multicast)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			for id, r := range m.remotes {
				if time.Since(r.seen) > 8*time.Second || !r.endpoint.ExpiresAt.After(time.Now()) {
					m.emit(DiscoveryEvent{Kind: "expired", EndpointID: id, DeviceID: r.endpoint.DeviceID, Protocol: "tcp", Port: r.proxy.Port()})
					r.proxy.Close()
					delete(m.remotes, id)
					continue
				}
				_, _ = m.emitter.WriteToUDP(Announcement(r.endpoint.MOTD, r.proxy.Port()), group)
			}
			m.mu.Unlock()
		}
	}
}

type Proxy struct {
	listener    net.Listener
	target      string
	mu          sync.Mutex
	connections map[net.Conn]bool
	closed      bool
	wg          sync.WaitGroup
}

func NewProxy(localIP, target string) (*Proxy, error) {
	if !LocalAddresses()[localIP] {
		return nil, errors.New("proxy must bind a local address")
	}
	l, err := net.Listen("tcp4", net.JoinHostPort(localIP, "0"))
	if err != nil {
		return nil, err
	}
	p := &Proxy{listener: l, target: target, connections: map[net.Conn]bool{}}
	p.wg.Add(1)
	go p.accept()
	return p, nil
}
func (p *Proxy) Port() uint16 { return uint16(p.listener.Addr().(*net.TCPAddr).Port) }
func (p *Proxy) accept() {
	defer p.wg.Done()
	for {
		c, err := p.listener.Accept()
		if err != nil {
			return
		}
		host, _, err := net.SplitHostPort(c.RemoteAddr().String())
		if err != nil || !LocalAddresses()[host] {
			c.Close()
			continue
		}
		p.mu.Lock()
		if p.closed || len(p.connections) >= 64 {
			p.mu.Unlock()
			c.Close()
			continue
		}
		p.connections[c] = true
		p.wg.Add(1)
		p.mu.Unlock()
		go p.forward(c)
	}
}
func (p *Proxy) forward(c net.Conn) {
	defer p.wg.Done()
	defer func() { c.Close(); p.mu.Lock(); delete(p.connections, c); p.mu.Unlock() }()
	remote, err := net.DialTimeout("tcp4", p.target, 3*time.Second)
	if err != nil {
		return
	}
	defer remote.Close()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.connections[remote] = true
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.connections, remote); p.mu.Unlock() }()
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(remote, c)
		if t, ok := remote.(*net.TCPConn); ok {
			_ = t.CloseWrite()
		}
		close(done)
	}()
	_, _ = io.Copy(c, remote)
	c.Close()
	remote.Close()
	<-done
}
func (p *Proxy) Close() {
	p.mu.Lock()
	p.closed = true
	p.listener.Close()
	for c := range p.connections {
		c.Close()
	}
	p.mu.Unlock()
	p.wg.Wait()
}
