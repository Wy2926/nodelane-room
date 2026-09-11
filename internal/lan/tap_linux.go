package lan

import (
	"crypto/sha256"
	"fmt"
	"net"
	"net/netip"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

func OpenTAP(name string, prefix netip.Prefix) (*TAP, error) {
	if len(name) > 15 {
		return nil, fmt.Errorf("TAP interface name too long")
	}
	// Stable for the quarantined IP allocation; no adapter MAC changes during
	// renewals or policy restarts, and no physical interface is ever attached.
	h := sha256.Sum256([]byte("nodelane-lan:" + prefix.Addr().String()))
	mac := net.HardwareAddr{2, h[0], h[1], h[2], h[3], h[4]}
	link := &netlink.Tuntap{LinkAttrs: netlink.LinkAttrs{Name: name, MTU: 1500, HardwareAddr: mac}, Mode: netlink.TUNTAP_MODE_TAP, Flags: netlink.TUNTAP_TUN_EXCL | netlink.TUNTAP_NO_PI | netlink.TUNTAP_ONE_QUEUE, Queues: 1, NonPersist: true}
	if err := netlink.LinkAdd(link); err != nil {
		return nil, err
	}
	if len(link.Fds) != 1 {
		return nil, fmt.Errorf("TAP did not return one packet queue")
	}
	f := link.Fds[0]
	if err := netlink.LinkSetHardwareAddr(link, mac); err != nil {
		f.Close()
		return nil, err
	}
	t := &TAP{ReadWriteCloser: f, MAC: mac, Name: name}
	t.Activate = func() error {
		for _, p := range tapAddresses(prefix, mac) {
			a := &netlink.Addr{IPNet: &net.IPNet{IP: net.IP(p.Addr().AsSlice()), Mask: net.CIDRMask(p.Bits(), p.Addr().BitLen())}, Flags: unix.IFA_F_NODAD}
			if err := netlink.AddrReplace(link, a); err != nil {
				return err
			}
		}
		return netlink.LinkSetUp(link)
	}
	return t, nil
}
