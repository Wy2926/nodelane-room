//go:build !linux && !windows

package platform

import (
	"context"
	"net"
)

func desktopStateDir() string                                     { return "" }
func secureDesktopState(string) error                             { return nil }
func listenDesktop(string) (net.Listener, bool, error)            { return nil, false, nil }
func dialDesktop(context.Context, string) (net.Conn, bool, error) { return nil, false, nil }
