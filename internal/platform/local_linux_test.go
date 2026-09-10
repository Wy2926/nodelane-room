//go:build linux

package platform

import (
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestUIDProbe(t *testing.T) {
	path := os.Getenv("NODELANE_UID_PROBE")
	if path == "" {
		return
	}
	c, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(time.Second))
	b, _ := io.ReadAll(c)
	if string(b) != os.Getenv("NODELANE_UID_EXPECT") {
		t.Fatalf("unexpected reply %q", b)
	}
}

func TestDesktopPeerUIDIsolation(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires isolated root Linux test environment")
	}
	dir, err := os.MkdirTemp("", "nlroom-uid-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err = os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agent.sock")
	socket, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	// Deliberately permit both test UIDs at the filesystem layer to prove that
	// SO_PEERCRED rejects a different user before HTTP can be read.
	os.Chmod(path, 0666)
	l := &uidListener{UnixListener: socket, owner: 65534}
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := l.Accept()
		if err == nil {
			c.Write([]byte("allowed"))
			c.Close()
		}
	}()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	exe = filepath.Join(dir, "probe")
	if err = os.WriteFile(exe, data, 0755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		uid   uint32
		reply string
	}{{65533, ""}, {65534, "allowed"}} {
		cmd := exec.Command(exe, "-test.run=^TestUIDProbe$")
		cmd.Env = append(os.Environ(), "NODELANE_UID_PROBE="+path, "NODELANE_UID_EXPECT="+tc.reply)
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: tc.uid, Gid: tc.uid}}
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("UID %d: %s %v", tc.uid, output, err)
		}
	}
	socket.Close()
	<-done
}
