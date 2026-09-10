//go:build linux

package engine

import (
	"fmt"
	"github.com/vishvananda/netlink"
	"net/netip"
)

func checkRoutes(network netip.Prefix, ownInterface string) error {
	rows, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.Dst == nil {
			continue
		}
		p, err := netip.ParsePrefix(r.Dst.String())
		if err != nil || p.Bits() == 0 {
			continue
		}
		if !network.Overlaps(p) {
			continue
		}
		i, err := netlink.LinkByIndex(r.LinkIndex)
		if err != nil {
			return err
		}
		if i.Attrs().Name != ownInterface {
			return fmt.Errorf("overlay %s overlaps route %s via %s", network, p, i.Attrs().Name)
		}
	}
	return nil
}
