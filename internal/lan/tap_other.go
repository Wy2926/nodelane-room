//go:build !windows && !linux

package lan

import (
	"github.com/nodelane/nodelane-room/internal/model"
	"net/netip"
)

func OpenTAP(string, netip.Prefix) (*TAP, error) {
	return nil, model.Failure("local_platform_unsupported")
}
