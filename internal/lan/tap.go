package lan

import (
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"

	"github.com/nodelane/nodelane-room/internal/model"
)

type TAP struct {
	io.ReadWriteCloser
	MAC       net.HardwareAddr
	Name      string
	Activate  func() error
	cleanup   func() error
	closeOnce sync.Once
	closeErr  error
}

func (t *TAP) Close() error {
	t.closeOnce.Do(func() {
		t.closeErr = t.ReadWriteCloser.Close()
		if t.cleanup != nil {
			t.closeErr = errors.Join(t.closeErr, t.cleanup())
		}
	})
	return t.closeErr
}

func tapAddresses(prefix netip.Prefix, mac net.HardwareAddr) []netip.Prefix {
	return []netip.Prefix{prefix, netip.PrefixFrom(model.LANIPv6(prefix.Addr()), 64), netip.PrefixFrom(model.LANLinkLocal(mac), 64)}
}
