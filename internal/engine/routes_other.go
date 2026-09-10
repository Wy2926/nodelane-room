//go:build !linux && !windows

package engine

import "net/netip"

func checkRoutes(netip.Prefix, string) error { return nil }
