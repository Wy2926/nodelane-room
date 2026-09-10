//go:build windows

package platform

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const pipeName = `\\.\pipe\NodeLaneRoom`
const serviceName = "NodeLaneRoom"

func DefaultDir() string { return filepath.Join(os.Getenv("ProgramData"), "NodeLaneRoom") }
func CurrentSID() (string, error) {
	t, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer t.Close()
	u, err := t.GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}
func SecureDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	base, err := filepath.Abs(os.Getenv("ProgramData"))
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Dir(abs), base) {
		return errors.New("Windows state directory must be directly under ProgramData")
	}
	// Create the directory with its final ACL/owner atomically. A directory
	// pre-created by an ordinary user cannot be adopted by a SYSTEM service:
	// retaining that owner would allow them to rewrite its DACL later.
	sd, err := windows.SecurityDescriptorFromString("O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	path, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	if err = windows.CreateDirectory(path, &sa); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	attrs, err := windows.GetFileAttributes(path)
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return errors.New("state path must be a real directory, not a reparse point")
	}
	current, err := windows.GetNamedSecurityInfo(abs, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := current.Owner()
	if err != nil {
		return err
	}
	if owner.String() != "S-1-5-18" && owner.String() != "S-1-5-32-544" {
		return errors.New("state directory has an untrusted owner; use a new administrator-created state directory")
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(abs, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
func protect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty secret")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_LOCAL_MACHINE|windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
func unprotect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty secret")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
func ListenLocal(dir string) (net.Listener, error) {
	b, err := os.ReadFile(filepath.Join(dir, "owner.sid"))
	if err != nil {
		return nil, fmt.Errorf("install the service first: %w", err)
	}
	sid := strings.TrimSpace(string(b))
	if _, err = windows.StringToSid(sid); err != nil {
		return nil, err
	}
	return winio.ListenPipe(pipeName, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;" + sid + ")", MessageMode: false, InputBufferSize: 65536, OutputBufferSize: 65536})
}
func DialLocal(ctx context.Context, _ string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, pipeName)
}
func Install(dir, ownerSID string) error {
	if err := SecureDir(dir); err != nil {
		return fmt.Errorf("run installation in an Administrator terminal: %w", err)
	}
	sid, err := CurrentSID()
	if err != nil {
		return err
	}
	if ownerSID != "" {
		if _, err = windows.StringToSid(ownerSID); err != nil {
			return err
		}
		sid = ownerSID
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// Service code must live in an administrator-controlled installation directory.
	programFiles, err := filepath.Abs(os.Getenv("ProgramFiles"))
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(exe)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(programFiles, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errors.New("install the release under Program Files before registering its SYSTEM service")
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if existing, e := m.OpenService(serviceName); e == nil {
		existing.Close()
		return errors.New("NodeLaneRoom service already exists")
	}
	if err = os.WriteFile(filepath.Join(dir, "owner.sid"), []byte(sid), 0600); err != nil {
		return err
	}
	s, err := m.CreateService(serviceName, exe, mgr.Config{DisplayName: "NodeLane Room", Description: "Nebula game room networking", StartType: mgr.StartAutomatic}, "daemon", "--state-dir", dir)
	if err != nil {
		return err
	}
	defer s.Close()
	_ = s.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}, {Type: mgr.ServiceRestart, Delay: 15 * time.Second}}, 60)
	return s.Start()
}
func Uninstall(_ string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer s.Close()
	status, err := s.Query()
	if err != nil {
		return err
	}
	if status.State != svc.Stopped {
		if _, err = s.Control(svc.Stop); err != nil {
			return err
		}
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			status, err = s.Query()
			if err != nil {
				return err
			}
			if status.State == svc.Stopped {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if status.State != svc.Stopped {
			return errors.New("service has not stopped; refusing to delete registration")
		}
	}
	return s.Delete()
}

type handler struct{ run func(context.Context) error }

func (h handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.run(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			if err != nil {
				return true, 1
			}
			return false, 0
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				err := <-done
				if err != nil {
					return true, 1
				}
				return false, 0
			}
		}
	}
}
func RunService(run func(context.Context) error) (bool, error) {
	is, err := svc.IsWindowsService()
	if err != nil || !is {
		return false, err
	}
	return true, svc.Run(serviceName, handler{run})
}
func EnsureForegroundOwner(dir string) error {
	if err := SecureDir(dir); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "owner.sid")); err == nil {
		return nil
	}
	sid, err := CurrentSID()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "owner.sid"), []byte(sid), 0600)
}
