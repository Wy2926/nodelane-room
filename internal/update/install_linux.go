//go:build linux

package update

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

func SupportedInstall(dir string) bool { return os.Geteuid() == 0 && dir == "/var/lib/nlroom" }
func StartInstall(ctx context.Context, dir string) error {
	if !SupportedInstall(dir) {
		return errors.New("update_install_unsupported")
	}
	return exec.CommandContext(ctx, "systemctl", "start", "--no-block", "nlroom-update.service").Run()
}
func Bootstrap(context.Context, string) error { return errors.New("update_install_unsupported") }
func finishInstall()                          {}
func installLock(dir string) (func(), error) {
	if !SupportedInstall(dir) {
		return nil, errors.New("update_install_unsupported")
	}
	f, e := os.OpenFile(filepath.Join(dir, "update.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("update_busy")
	}
	return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN); f.Close() }, nil
}
func FreeSpace(dir string) (uint64, error) {
	var s unix.Statfs_t
	err := unix.Statfs(dir, &s)
	return s.Bavail * uint64(s.Bsize), err
}

func aptInstall(ctx context.Context, file, version string) error {
	// Only this package and its required dependencies; never remove packages or
	// run a system-wide upgrade. APT owns its own global lock throughout install.
	info, err := exec.CommandContext(ctx, "dpkg-deb", "--show", "--showformat=${Package} ${Version} ${Architecture}", file).Output()
	if err != nil || strings.TrimSpace(string(info)) != "nlroom "+version+" "+runtime.GOARCH {
		return errors.New("update_package_invalid")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Once dpkg starts, wait for it to finish. Killing its parent on a timeout can
	// leave package scripts running concurrently with recovery.
	cmd := exec.Command("apt-get", "-y", "--no-remove", "--allow-downgrades", "-o", "DPkg::Lock::Timeout=60", "install", file)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	return cmd.Run()
}
func installPackage(ctx context.Context, dir string, j InstallJob) error {
	if j.Previous == nil {
		return errors.New("update_rollback_unavailable")
	}
	if err := exec.CommandContext(ctx, "systemctl", "stop", "nlroom-service.service").Run(); err != nil {
		return err
	}
	b, err := exec.CommandContext(ctx, "systemctl", "show", "-p", "MainPID", "--value", "nlroom-service.service").Output()
	if err != nil || strings.TrimSpace(string(b)) != "0" {
		return errors.New("update_stop_failed")
	}
	return aptInstall(ctx, PackagePath(dir, j.Release.UpdateArtifact), j.Release.Version)
}
func rollbackPackage(ctx context.Context, dir string, j InstallJob) error {
	if j.Previous == nil {
		return errors.New("update_rollback_unavailable")
	}
	if err := VerifyPrepared(dir, *j.Previous); err != nil {
		return err
	}
	return aptInstall(ctx, PackagePath(dir, j.Previous.Release.UpdateArtifact), j.From)
}
