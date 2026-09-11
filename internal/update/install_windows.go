//go:build windows

package update

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func SupportedInstall(dir string) bool {
	return filepath.Clean(dir) == filepath.Join(os.Getenv("ProgramData"), "NodeLaneRoom")
}

// Windows installation is explicitly launched by the ordinary user's GUI with
// runas. The service only stages a protected job, never opens a SYSTEM desktop.
func StartInstall(context.Context, string) error { return errors.New("update_elevation_required") }
func finishInstall() {
	cmd := exec.Command("schtasks.exe", "/Delete", "/TN", "NodeLaneRoomUpdateRecovery", "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
}
func Bootstrap(_ context.Context, dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	in, err := os.Open(exe)
	if err != nil {
		return err
	}
	defer in.Close()
	name := filepath.Join(dir, "update-runner.exe")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, in)
	if err == nil {
		err = f.Sync()
	}
	f.Close()
	if err != nil {
		return err
	}
	// A boot task resumes only this protected, signed job after a power loss.
	recovery := exec.Command("schtasks.exe", "/Create", "/TN", "NodeLaneRoomUpdateRecovery", "/TR", `"`+name+`" apply`, "/SC", "ONSTART", "/RU", "SYSTEM", "/RL", "HIGHEST", "/F")
	recovery.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err = recovery.Run(); err != nil {
		return errors.New("update_recovery_failed")
	}
	cmd := exec.Command(name, "apply")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	if err = cmd.Start(); err == nil {
		err = cmd.Process.Release()
	}
	return err
}
func installLock(dir string) (func(), error) {
	if !SupportedInstall(dir) {
		return nil, errors.New("update_install_unsupported")
	}
	p, err := windows.UTF16PtrFromString(filepath.Join(dir, "update.lock"))
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, errors.New("update_busy")
	}
	return func() { windows.CloseHandle(h) }, nil
}
func FreeSpace(dir string) (uint64, error) {
	p, e := windows.UTF16PtrFromString(dir)
	if e != nil {
		return 0, e
	}
	var free uint64
	e = windows.GetDiskFreeSpaceEx(p, &free, nil, nil)
	return free, e
}
func installPackage(ctx context.Context, dir string, j InstallJob) error {
	// Let the elevated bootstrap process release the installed updater binary.
	t := time.NewTimer(2 * time.Second)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
	}
	// Do not kill NSIS while its child installer is still changing the service.
	cmd := exec.Command(PackagePath(dir, j.Release.UpdateArtifact), "/S", "/MANAGEDUPDATE")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
func rollbackPackage(ctx context.Context, dir string, j InstallJob) error {
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	err := WaitReady(probe, dir, j.From)
	cancel()
	if err == nil {
		return nil
	}
	// Reuse the same verified installer if the service failed after NSIS's own
	// health check. Its rollback path validates the protected previous bundle.
	b, err := os.ReadFile(filepath.Join(os.Getenv("ProgramFiles"), "NodeLaneRoom.previous", "BUILD.txt"))
	if err != nil {
		return errors.New("update_rollback_unavailable")
	}
	version := ""
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "Version: ") {
			version = strings.TrimSpace(strings.TrimPrefix(line, "Version: "))
		}
	}
	if version != j.From {
		return errors.New("update_rollback_invalid")
	}
	if err = VerifyPrepared(dir, j.Prepared); err != nil {
		return err
	}
	cmd := exec.Command(PackagePath(dir, j.Release.UpdateArtifact), "/S", "/MANAGEDUPDATE", "/ROLLBACK")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
