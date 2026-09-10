//go:build linux

package platform

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const desktopSocket = "/run/nlroom/agent.sock"

func desktopStateDir() string { return "/var/lib/nlroom" }

func rootPath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 || info.IsDir() != directory {
		return fmt.Errorf("unsafe root-owned path: %s", path)
	}
	if directory && path != "/" {
		return rootPath(filepath.Dir(path), true)
	}
	return nil
}

func secureDesktopState(dir string) error {
	if os.Geteuid() != 0 {
		return errors.New("desktop networking service must run as root")
	}
	if err := rootPath(filepath.Dir(dir), true); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	if err := rootPath(dir, true); err != nil {
		return err
	}
	return os.Chmod(dir, 0700)
}

func listenDesktop(dir string) (net.Listener, bool, error) {
	if dir != desktopStateDir() {
		return nil, false, nil
	}
	if err := secureDesktopState(dir); err != nil {
		return nil, true, err
	}
	const config = "/etc/nlroom/owner.uid"
	if err := rootPath(filepath.Dir(config), true); err != nil {
		return nil, true, err
	}
	if err := rootPath(config, false); err != nil {
		return nil, true, err
	}
	b, err := os.ReadFile(config)
	if err != nil {
		return nil, true, err
	}
	uid, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 32)
	if err != nil || uid == 0 {
		return nil, true, errors.New("configure a non-root installation owner UID")
	}
	if err = rootPath(filepath.Dir(desktopSocket), true); err != nil {
		return nil, true, err
	}
	if c, err := net.Dial("unix", desktopSocket); err == nil {
		c.Close()
		return nil, true, errors.New("agent already running")
	}
	if info, err := os.Lstat(desktopSocket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, true, errors.New("desktop socket path is not a socket")
		}
		if err = os.Remove(desktopSocket); err != nil {
			return nil, true, err
		}
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: desktopSocket, Net: "unix"})
	if err != nil {
		return nil, true, err
	}
	if err = os.Chown(desktopSocket, int(uid), 0); err == nil {
		err = os.Chmod(desktopSocket, 0600)
	}
	if err != nil {
		l.Close()
		return nil, true, err
	}
	return &uidListener{UnixListener: l, owner: uint32(uid)}, true, nil
}

type uidListener struct {
	*net.UnixListener
	owner uint32
}

func peerUID(c *net.UnixConn) (uint32, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	var uid uint32
	var sockErr error
	err = raw.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		sockErr = e
		if e == nil {
			uid = cred.Uid
		}
	})
	if err != nil {
		return 0, err
	}
	return uid, sockErr
}

func (l *uidListener) Accept() (net.Conn, error) {
	for {
		c, err := l.AcceptUnix()
		if err != nil {
			return nil, err
		}
		uid, err := peerUID(c)
		if err == nil && (uid == l.owner || uid == 0) {
			return c, nil
		}
		c.Close()
	}
}

func dialDesktop(ctx context.Context, dir string) (net.Conn, bool, error) {
	if dir != desktopStateDir() {
		return nil, false, nil
	}
	c, err := (&net.Dialer{}).DialContext(ctx, "unix", desktopSocket)
	if err != nil {
		return nil, true, err
	}
	uid, err := peerUID(c.(*net.UnixConn))
	if err != nil || uid != 0 {
		c.Close()
		return nil, true, errors.New("local service is not root-owned")
	}
	return c, true, nil
}
