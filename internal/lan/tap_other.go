//go:build !windows && !linux

package lan

import (
	"errors"
	"net/netip"
)

func OpenTAP(string, netip.Prefix) (*TAP, error) {
	return nil, errors.New("Ethernet LAN is supported on Windows and Linux only")
}
