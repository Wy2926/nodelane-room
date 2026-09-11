package lan

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

type probeDatagram struct {
	data []byte
	ip   netip.Addr
}
type probeConn struct {
	device *Device
	done   chan struct{}
	once   sync.Once
}

func (d *Device) ProbeConn() net.PacketConn { return d.probe }
func (p *probeConn) Close() error           { p.once.Do(func() { close(p.done) }); return nil }
func (p *probeConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IP(p.device.ip.AsSlice()), Port: model.ProbePort}
}
func (p *probeConn) SetDeadline(t time.Time) error {
	if !t.IsZero() {
		return errors.ErrUnsupported
	}
	return nil
}
func (p *probeConn) SetReadDeadline(t time.Time) error  { return p.SetDeadline(t) }
func (p *probeConn) SetWriteDeadline(t time.Time) error { return p.SetDeadline(t) }
func (p *probeConn) ReadFrom(b []byte) (int, net.Addr, error) {
	select {
	case <-p.done:
		return 0, nil, net.ErrClosed
	case <-p.device.closed:
		return 0, nil, net.ErrClosed
	case v := <-p.device.probeIn:
		return copy(b, v.data), &net.UDPAddr{IP: net.IP(v.ip.AsSlice()), Port: model.ProbePort}, nil
	}
}
func (p *probeConn) WriteTo(b []byte, to net.Addr) (int, error) {
	addr, ok := to.(*net.UDPAddr)
	if !ok || addr.Port != model.ProbePort || len(b) > 1024 {
		return 0, errors.New("invalid diagnostic datagram")
	}
	ip, ok := netip.AddrFromSlice(addr.IP)
	if !ok {
		return 0, errors.New("invalid diagnostic address")
	}
	ip = ip.Unmap()
	d := p.device
	d.mu.Lock()
	allowed := d.active(d.members[d.ip], time.Now()) && (d.nodes[ip] || d.active(d.members[ip], time.Now()))
	d.mu.Unlock()
	if !allowed {
		return 0, errors.New("diagnostic peer unavailable")
	}
	select {
	case <-p.done:
		return 0, net.ErrClosed
	case <-d.closed:
		return 0, net.ErrClosed
	default:
	}
	select {
	case d.probeOut <- udpPacket(d.ip, ip, model.ProbePort, bytes.Clone(b)):
		return len(b), nil
	default:
		return 0, io.ErrShortWrite
	}
}

type tapRead struct {
	data []byte
	err  error
}

func (d *Device) readTAP() {
	defer close(d.tapDone)
	b := make([]byte, maxFrame+1)
	for {
		n, err := d.tap.Read(b)
		v := tapRead{err: err}
		if err == nil {
			v.data = bytes.Clone(b[:n])
		}
		select {
		case <-d.closed:
			return
		case d.tapIn <- v:
		}
		if err != nil {
			return
		}
	}
}
