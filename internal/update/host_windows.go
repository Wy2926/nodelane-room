package update

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/nodelane/nodelane-room/internal/platform"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\NodeLaneRoom`

func windowsInstallDir() (string, error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "NodeLaneRoom"), nil
}

func checkWindowsTree(path string, trusted bool) error {
	return filepath.WalkDir(path, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		p, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		attrs, err := windows.GetFileAttributes(p)
		if err != nil {
			return err
		}
		if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("refusing installation reparse point")
		}
		if !trusted {
			return nil
		}
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			return err
		}
		owner, _, err := sd.Owner()
		if err != nil {
			return err
		}
		if owner == nil || (owner.String() != "S-1-5-18" && owner.String() != "S-1-5-32-544") {
			return errors.New("installation path has an untrusted owner")
		}
		acl, _, err := sd.DACL()
		if err != nil {
			return err
		}
		if acl == nil {
			return errors.New("installation path has no access restrictions")
		}
		for i := uint32(0); i < uint32(acl.AceCount); i++ {
			var ace *windows.ACCESS_ALLOWED_ACE
			if err := windows.GetAce(acl, i, &ace); err != nil {
				return err
			}
			if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
				continue
			}
			if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
				return errors.New("unsupported installation access rule")
			}
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
			if sid != "S-1-5-18" && sid != "S-1-5-32-544" && sid != "S-1-3-0" && uint32(ace.Mask)&(0xD0156|windows.GENERIC_WRITE|windows.GENERIC_ALL) != 0 {
				return errors.New("installation path is writable by another user")
			}
		}
		return nil
	})
}

func protectBundlePath(path string) error {
	sd, err := windows.SecurityDescriptorFromString("O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;BU)")
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, acl, nil)
}

func createBundleDirectory(path string) error {
	sd, err := windows.SecurityDescriptorFromString("O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;BU)")
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	return windows.CreateDirectory(p, &sa)
}

func createBundleTemp(dir string) (*os.File, error) {
	return createSetupTemp(dir, "O:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;GRGX;;;BU)")
}

func createSetupTemp(dir, descriptor string) (*os.File, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	name := filepath.Join(dir, fmt.Sprintf(".install-%x", id))
	sd, err := windows.SecurityDescriptorFromString(descriptor)
	if err != nil {
		return nil, err
	}
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, 0, &sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), name), nil
}

// Create every directory with its final owner and ACL. A power loss before the
// first file is written must not leave a directory that a repair later rejects.
func prepareBundleDirectories(target string, b bundle) error {
	paths := map[string]bool{target: true}
	for name := range b.files {
		for dir := filepath.Dir(filepath.Join(target, filepath.FromSlash(name))); dir != target; dir = filepath.Dir(dir) {
			paths[dir] = true
		}
	}
	names := make([]string, 0, len(paths))
	for path := range paths {
		names = append(names, path)
	}
	slices.Sort(names)
	for _, path := range names {
		if err := createBundleDirectory(path); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return err
		}
	}
	return nil
}

func lockWindowsFile(path string) (func(), error) {
	if err := checkWindowsTree(path, true); err != nil {
		return nil, err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString("O:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)")
	if err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, &sa, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, errors.New("another installation or update is active")
	}
	return func() { windows.CloseHandle(h) }, nil
}

func openInstalledService(target string) (*mgr.Service, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, err
	}
	defer m.Disconnect()
	s, err := m.OpenService("NodeLaneRoom")
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cfg, err := s.Config()
	if err == nil {
		var args []string
		args, err = windows.DecomposeCommandLine(cfg.BinaryPathName)
		if err == nil && (len(args) == 0 || !strings.EqualFold(args[0], filepath.Join(target, "nlroom-service.exe")) || !strings.EqualFold(cfg.ServiceStartName, "LocalSystem")) {
			err = errors.New("unexpected installed service configuration")
		}
	}
	if err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func waitService(ctx context.Context, s *mgr.Service, state svc.State) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		status, err := s.Query()
		if err != nil {
			return err
		}
		if status.State == state {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("service did not reach the expected state")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func stopInstalledService(ctx context.Context, target string) error {
	s, err := openInstalledService(target)
	if err != nil {
		return err
	}
	if s != nil {
		defer s.Close()
		status, err := s.Query()
		if err != nil {
			return err
		}
		var process windows.Handle
		if status.ProcessId != 0 {
			process, err = windows.OpenProcess(windows.SYNCHRONIZE, false, status.ProcessId)
			if err != nil && !errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
				return err
			}
			if process != 0 {
				defer windows.CloseHandle(process)
			}
		}
		if status.State != svc.Stopped && status.State != svc.StopPending {
			if _, err = s.Control(svc.Stop); err != nil {
				return err
			}
		}
		if err := waitService(ctx, s, svc.Stopped); err != nil {
			return err
		}
		if process != 0 {
			result, err := windows.WaitForSingleObject(process, 10000)
			if err != nil {
				return err
			}
			if result != windows.WAIT_OBJECT_0 {
				return errors.New("network service process has not exited")
			}
		}
	}
	return waitInstalledProcesses(target, "nlroom-service.exe", false)
}

func waitInstalledProcesses(target, name string, terminate bool) error {
	pids := make([]uint32, 4096)
	var used uint32
	for {
		if err := windows.EnumProcesses(pids, &used); err != nil {
			return err
		}
		if int(used) < len(pids)*4 {
			break
		}
		pids = make([]uint32, len(pids)*2)
	}
	for _, pid := range pids[:used/4] {
		if pid == 0 || pid == uint32(os.Getpid()) {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
		if err != nil {
			continue
		}
		buf := make([]uint16, 32768)
		size := uint32(len(buf))
		err = windows.QueryFullProcessImageName(h, 0, &buf[0], &size)
		if err != nil || !strings.EqualFold(windows.UTF16ToString(buf[:size]), filepath.Join(target, name)) {
			windows.CloseHandle(h)
			continue
		}
		if terminate {
			kill, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
			if err != nil {
				windows.CloseHandle(h)
				return err
			}
			err = windows.TerminateProcess(kill, 0)
			windows.CloseHandle(kill)
			if err != nil {
				windows.CloseHandle(h)
				return err
			}
		}
		result, err := windows.WaitForSingleObject(h, 10000)
		windows.CloseHandle(h)
		if err != nil {
			return err
		}
		if result != windows.WAIT_OBJECT_0 {
			return fmt.Errorf("%s has not exited", name)
		}
	}
	return nil
}

func activateBundle(ctx context.Context, target, owner, version string) error {
	s, err := openInstalledService(target)
	if err != nil {
		return err
	}
	if s != nil {
		defer s.Close()
	}
	b, err := readBundle(target)
	if err != nil {
		return err
	}
	if b.version != version {
		return errors.New("installed version mismatch")
	}
	if s == nil {
		if err := writePrivateText(filepath.Join(platform.DefaultDir(), "owner.sid"), owner); err != nil {
			return err
		}
		cmd := hiddenCommand(filepath.Join(target, "nlroom-service.exe"), "service", "install", "--owner-sid", owner)
		if err := cmd.Run(); err != nil {
			return errors.New("service registration failed")
		}
	} else {
		status, err := s.Query()
		if err != nil {
			return err
		}
		if status.State != svc.Running {
			if err := s.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
				return err
			}
		}
	}
	if err := WaitReady(ctx, platform.DefaultDir(), version); err != nil {
		return err
	}
	if b.gui {
		return registerBundle(target, version)
	}
	return nil
}

func registerBundle(target, version string) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, uninstallKey, registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer k.Close()
	for name, value := range map[string]string{"DisplayName": "NodeLane Room", "DisplayVersion": version, "Publisher": "NodeLane", "InstallLocation": target, "DisplayIcon": filepath.Join(target, "nlroom.exe"), "UninstallString": windows.ComposeCommandLine([]string{filepath.Join(target, "Uninstall.exe")}), "QuietUninstallString": windows.ComposeCommandLine([]string{filepath.Join(target, "Uninstall.exe"), "/S"})} {
		if err := k.SetStringValue(name, value); err != nil {
			return err
		}
	}
	for _, name := range []string{"NoModify", "NoRepair"} {
		if err := k.SetDWordValue(name, 1); err != nil {
			return err
		}
	}
	return createShortcut(target)
}

func unregisterBundle() error {
	if err := registry.DeleteKey(registry.LOCAL_MACHINE, uninstallKey); err != nil && !errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return err
	}
	programs, err := windows.KnownFolderPath(windows.FOLDERID_CommonPrograms, 0)
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(programs, "NodeLane Room.lnk"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func createShortcut(target string) error {
	programs, err := windows.KnownFolderPath(windows.FOLDERID_CommonPrograms, 0)
	if err != nil {
		return err
	}
	return writeShortcut(filepath.Join(programs, "NodeLane Room.lnk"), target)
}

func writeShortcut(destination, target string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		return err
	}
	defer windows.CoUninitialize()
	class := windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iid := windows.GUID{Data1: 0x000214f9, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	type object struct{ methods *[21]uintptr }
	var link *object
	proc := windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
	result, _, _ := proc.Call(uintptr(unsafe.Pointer(&class)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&link)))
	if int32(result) < 0 {
		return errors.New("could not create Start menu shortcut")
	}
	defer syscall.SyscallN(link.methods[2], uintptr(unsafe.Pointer(link)))
	path, err := windows.UTF16PtrFromString(filepath.Join(target, "nlroom.exe"))
	if err != nil {
		return err
	}
	result, _, _ = syscall.SyscallN(link.methods[20], uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(path)))
	if int32(result) < 0 {
		return errors.New("could not set shortcut target")
	}
	working, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	result, _, _ = syscall.SyscallN(link.methods[9], uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(working)))
	if int32(result) < 0 {
		return errors.New("could not set shortcut directory")
	}
	iid = windows.GUID{Data1: 0x0000010b, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	var persist *object
	result, _, _ = syscall.SyscallN(link.methods[0], uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&persist)))
	if int32(result) < 0 {
		return errors.New("could not save shortcut")
	}
	defer syscall.SyscallN(persist.methods[2], uintptr(unsafe.Pointer(persist)))
	name, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	result, _, _ = syscall.SyscallN(persist.methods[6], uintptr(unsafe.Pointer(persist)), uintptr(unsafe.Pointer(name)), 1)
	if int32(result) < 0 {
		return errors.New("could not save Start menu shortcut")
	}
	return nil
}
