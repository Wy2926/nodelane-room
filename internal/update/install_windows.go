//go:build windows

package update

import (
	"context"
	"github.com/nodelane/nodelane-room/internal/model"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func SupportedInstall(dir string) bool {
	return filepath.Clean(dir) == filepath.Join(os.Getenv("ProgramData"), "NodeLaneRoom")
}

// Windows installation is explicitly launched by the ordinary user's GUI with
// runas. The service only stages a protected job, never opens a SYSTEM desktop.
func StartInstall(context.Context, string) error { return model.Failure("local_update_not_ready") }
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
	cmd := exec.Command(name, "apply")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	if err = cmd.Start(); err == nil {
		err = cmd.Process.Release()
	}
	return err
}
func installLock(dir string) (func(), error) {
	if !SupportedInstall(dir) {
		return nil, model.Failure("local_update_install_unsupported")
	}
	p, err := windows.UTF16PtrFromString(filepath.Join(dir, "update.lock"))
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, model.Failure("local_update_install_busy")
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
func installFailure(context.Context, string, InstallJob) (string, string) {
	return "failed", "local_update_install_failed"
}
