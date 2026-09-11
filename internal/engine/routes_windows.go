//go:build windows

package engine

import (
	"github.com/nodelane/nodelane-room/internal/model"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
	"net"
	"net/netip"
)

func checkRoutes(network netip.Prefix, ownInterface string) error {
	rows, err := winipcfg.GetIPForwardTable2(windows.AF_INET)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.DestinationPrefix.PrefixLength == 0 {
			continue
		}
		i, err := net.InterfaceByIndex(int(r.InterfaceIndex))
		if err != nil {
			return err
		}
		if i.Name == ownInterface {
			continue
		}
		p := netip.PrefixFrom(r.DestinationPrefix.RawPrefix.Addr(), int(r.DestinationPrefix.PrefixLength))
		if network.Overlaps(p) {
			return model.Failure("local_route_conflict")
		}
	}
	return nil
}
