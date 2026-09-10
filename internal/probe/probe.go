// Package probe measures the overlay and transports bounded discovery notices.
// All packets use Nebula's authenticated, encrypted overlay, never the public socket.
package probe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

type Packet struct {
	Version  int    `json:"v"`
	Type     string `json:"type"`
	Nonce    string `json:"nonce,omitempty"`
	Room     string `json:"room,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}
type pending struct {
	ip   string
	done chan struct{}
}
type sample struct {
	at  time.Time
	rtt time.Duration
	ok  bool
}
type bucket struct {
	second int64
	count  int
}
type Service struct {
	conn            *net.UDPConn
	mu              sync.Mutex
	allowed         map[string]bool
	room            string
	network         netip.Prefix
	infrastructure  bool
	pending         map[string]pending
	samples         map[string][]sample
	rate            map[string]bucket
	OnAdvertisement func(ip, room, endpoint string)
	done            chan struct{}
}

func Start(ip, network string, infrastructure bool, onAd func(string, string, string)) (*Service, error) {
	addr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(ip, "4243"))
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}
	n, err := netip.ParsePrefix(network)
	if err != nil {
		conn.Close()
		return nil, err
	}
	s := &Service{conn: conn, network: n, infrastructure: infrastructure, allowed: map[string]bool{}, pending: map[string]pending{}, samples: map[string][]sample{}, rate: map[string]bucket{}, OnAdvertisement: onAd, done: make(chan struct{})}
	go s.read()
	return s, nil
}
func (s *Service) Update(snapshot model.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowed = map[string]bool{}
	for _, m := range snapshot.Members {
		s.allowed[m.IP] = true
	}
	for _, n := range snapshot.Nodes {
		s.allowed[n.IP] = true
	}
	s.room = ""
	if snapshot.Room != nil {
		s.room = snapshot.Room.ID
	}
	for ip := range s.rate {
		if !s.allowed[ip] {
			delete(s.rate, ip)
		}
	}
}
func (s *Service) Close() { _ = s.conn.Close(); <-s.done }
func (s *Service) read() {
	defer close(s.done)
	buf := make([]byte, 1025)
	for {
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if n > 1024 || addr.Port != model.ProbePort {
			continue
		}
		ip := addr.IP.String()
		a, err := netip.ParseAddr(ip)
		if err != nil {
			continue
		}
		s.mu.Lock()
		allowed := s.allowed[ip] || s.infrastructure && s.network.Contains(a)
		b := s.rate[ip]
		second := time.Now().Unix()
		if b.second != second {
			b = bucket{second: second}
		}
		b.count++
		if len(s.rate) < 65536 || s.rate[ip].second != 0 {
			s.rate[ip] = b
		}
		room := s.room
		s.mu.Unlock()
		if !allowed || b.count > 10 {
			continue
		}
		var p Packet
		if json.Unmarshal(buf[:n], &p) != nil || p.Version != 1 {
			continue
		}
		switch p.Type {
		case "ping":
			if len(p.Nonce) != 32 {
				continue
			}
			p.Type = "pong"
			_ = s.send(ip, p)
		case "pong":
			s.mu.Lock()
			v, ok := s.pending[p.Nonce]
			if ok && v.ip == ip {
				delete(s.pending, p.Nonce)
				close(v.done)
			}
			s.mu.Unlock()
		case "advertisement":
			if !s.infrastructure && room != "" && p.Room == room && len(p.Endpoint) == 32 && s.OnAdvertisement != nil {
				s.OnAdvertisement(ip, p.Room, p.Endpoint)
			}
		}
	}
}
func (s *Service) send(ip string, p Packet) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.conn.WriteToUDP(b, &net.UDPAddr{IP: net.ParseIP(ip), Port: model.ProbePort})
	return err
}
func (s *Service) Ping(ctx context.Context, ip string) (time.Duration, error) {
	s.mu.Lock()
	allowed := s.allowed[ip] || s.infrastructure
	s.mu.Unlock()
	if !allowed {
		return 0, errors.New("peer is not authorized")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return 0, err
	}
	nonce := hex.EncodeToString(b)
	done := make(chan struct{})
	s.mu.Lock()
	s.pending[nonce] = pending{ip: ip, done: done}
	s.mu.Unlock()
	start := time.Now()
	ok := false
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.pending, nonce)
		v := append(s.samples[ip], sample{rtt: time.Since(start), ok: ok, at: time.Now().UTC()})
		if len(v) > 20 {
			v = v[len(v)-20:]
		}
		s.samples[ip] = v
	}()
	if err := s.send(ip, Packet{Version: 1, Type: "ping", Nonce: nonce}); err != nil {
		return 0, err
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-timer.C:
		return 0, errors.New("overlay probe timed out")
	case <-s.done:
		return 0, errors.New("probe service stopped")
	case <-done:
		ok = true
		return time.Since(start), nil
	}
}
func (s *Service) Advertise(ip, endpoint string) error {
	s.mu.Lock()
	room := s.room
	allowed := s.allowed[ip]
	s.mu.Unlock()
	if !allowed {
		return errors.New("peer is not authorized")
	}
	return s.send(ip, Packet{Version: 1, Type: "advertisement", Room: room, Endpoint: endpoint})
}
func (s *Service) Stats(ip string) (*float64, *float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.samples[ip]
	if len(v) == 0 {
		return nil, nil
	}
	var total time.Duration
	success := 0
	for _, x := range v {
		if x.ok {
			success++
			total += x.rtt
		}
	}
	loss := 100 * float64(len(v)-success) / float64(len(v))
	if success == 0 {
		return nil, &loss
	}
	rtt := float64(total/time.Duration(success)) / float64(time.Millisecond)
	return &rtt, &loss
}

// Failed requires consecutive measured failures; a first handshake timeout
// alone must not evict the only usable relay.
func (s *Service) Failed(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.samples[ip]
	if len(v) < 3 {
		return false
	}
	for _, x := range v[len(v)-3:] {
		if x.ok {
			return false
		}
	}
	return true
}

func (s *Service) LastSample(ip string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.samples[ip]
	if len(v) == 0 {
		return time.Time{}
	}
	return v[len(v)-1].at
}

func (s *Service) LastSuccess(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.samples[ip]
	return len(v) > 0 && v[len(v)-1].ok
}
