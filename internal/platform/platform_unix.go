//go:build !windows

package platform

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
)

func DefaultDir() string {
	if dir := desktopStateDir(); dir != "" {
		return dir
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ".nodelane"
	}
	return filepath.Join(dir, "nodelane")
}
func SecureDir(dir string) error {
	if dir == desktopStateDir() && dir != "" {
		return secureDesktopState(dir)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.Chmod(dir, 0700)
}
func protect(b []byte) ([]byte, error)   { return b, nil }
func unprotect(b []byte) ([]byte, error) { return b, nil }
func ListenLocal(dir string) (net.Listener, error) {
	if l, handled, err := listenDesktop(dir); handled {
		return l, err
	}
	if err := SecureDir(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "agent.sock")
	if c, err := net.Dial("unix", path); err == nil {
		c.Close()
		return nil, errors.New("agent already running")
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("socket path is not a socket")
		}
		if err = os.Remove(path); err != nil {
			return nil, err
		}
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}
func DialLocal(ctx context.Context, dir string) (net.Conn, error) {
	if c, handled, err := dialDesktop(ctx, dir); handled {
		return c, err
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dir, "agent.sock"))
}
func Install(string, string) error {
	return errors.New("automatic service installation is supported on Windows; use the supplied systemd units on Linux")
}
func Uninstall(string) error                               { return errors.New("use systemctl to stop and disable the Linux unit") }
func RunService(func(context.Context) error) (bool, error) { return false, nil }
func EnsureForegroundOwner(dir string) error               { return SecureDir(dir) }
