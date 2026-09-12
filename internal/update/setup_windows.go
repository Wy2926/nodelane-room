package update

import (
	"context"
	"crypto/rand"
	"debug/pe"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type setupOptions struct {
	action, source, owner, pipe string
	managed, purge              bool
}

// SetupCommand is used by NSIS and by the installed desktop at startup. Only
// setup/remove/tap are accepted; the UI cannot ask this helper to run a command.
func SetupCommand(ctx context.Context, args []string) (bool, error) {
	if len(args) == 0 || (args[0] != "setup" && args[0] != "remove" && args[0] != "tap") {
		return false, nil
	}
	o := setupOptions{action: args[0]}
	f := flag.NewFlagSet(o.action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.source, "source", "", "bundle source")
	f.StringVar(&o.owner, "owner-sid", "", "installation user")
	f.StringVar(&o.pipe, "log-pipe", "", "native status pipe")
	f.BoolVar(&o.managed, "managed", false, "verified online update")
	f.BoolVar(&o.purge, "purge", false, "clear local identity")
	if err := f.Parse(args[1:]); err != nil {
		return true, errors.New("invalid native setup arguments")
	}
	if f.NArg() != 0 || (o.action != "setup" && (o.source != "" || o.managed)) || (o.purge && o.action != "remove") {
		return true, errors.New("invalid setup operation")
	}
	exe, err := os.Executable()
	if err != nil {
		return true, err
	}
	target, err := windowsInstallDir()
	if err != nil {
		return true, err
	}
	if o.action == "remove" && strings.EqualFold(filepath.Dir(exe), target) {
		return true, errors.New("run Uninstall.exe, or run this helper from outside the installation directory")
	}
	if o.source == "" {
		o.source = filepath.Dir(exe)
	}
	o.source, err = filepath.Abs(o.source)
	if err != nil {
		return true, err
	}
	if o.action == "tap" {
		if !strings.EqualFold(filepath.Dir(exe), target) {
			return true, errors.New("TAP setup requires the installed client")
		}
		if guid, err := installedTAP(); err != nil || guid != "" {
			return true, err
		}
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return true, err
	}
	elevated := token.IsElevated()
	token.Close()
	if !elevated {
		if o.owner != "" || o.pipe != "" || o.managed {
			return true, errors.New("privileged setup options require administrator authorization")
		}
		o.owner, err = platform.CurrentSID()
		if err != nil {
			return true, err
		}
		return true, elevateSetup(exe, o, os.Stdout)
	}
	var output io.Writer = os.Stdout
	if o.pipe != "" {
		if !regexp.MustCompile(`^NodeLaneRoom-install-[0-9a-f]{32}$`).MatchString(o.pipe) {
			return true, errors.New("invalid setup status pipe")
		}
		pipeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		conn, err := winio.DialPipeContext(pipeCtx, `\\.\pipe\`+o.pipe)
		cancel()
		if err != nil {
			return true, err
		}
		defer conn.Close()
		output = conn
	}
	err = runWindowsSetup(ctx, target, o, output)
	if err != nil {
		fmt.Fprintln(output, "Installation operation failed:", err)
	}
	return true, err
}

func runWindowsSetup(ctx context.Context, target string, o setupOptions, output io.Writer) error {
	if err := platform.SecureDir(platform.DefaultDir()); err != nil {
		return err
	}
	if err := checkWindowsTree(platform.DefaultDir(), true); err != nil {
		return err
	}
	// Match the updater's lock order. Uninstall must not stop a service while an
	// independent updater is still using it or waiting for the directory lock.
	if !o.managed {
		unlock, err := lockWindowsFile(filepath.Join(platform.DefaultDir(), "update.lock"))
		if err != nil {
			return err
		}
		defer unlock()
	}
	unlock, err := lockWindowsFile(filepath.Join(platform.DefaultDir(), "install.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	if err := checkWindowsTree(target, true); err != nil {
		return err
	}
	ownerPath := filepath.Join(platform.DefaultDir(), "owner.sid")
	stored, readErr := os.ReadFile(ownerPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if o.managed {
		job, err := LoadJob(platform.DefaultDir())
		if err != nil {
			return err
		}
		if (job.State != "installing" && job.State != "ready") || job.Release.Version != model.ClientVersion || VerifyPrepared(platform.DefaultDir(), job.Prepared) != nil {
			return errors.New("managed update is not verified")
		}
		o.owner = strings.TrimSpace(string(stored))
	}
	if o.owner == "" {
		o.owner, err = platform.CurrentSID()
		if err != nil {
			return err
		}
	}
	if _, err := windows.StringToSid(o.owner); err != nil {
		return errors.New("invalid installation user")
	}
	if o.action != "remove" && readErr == nil && strings.TrimSpace(string(stored)) != o.owner {
		return errors.New("run the client or installer from the original player account")
	}
	if o.action == "tap" {
		if readErr != nil {
			return errors.New("install the network service before preparing TAP")
		}
		fmt.Fprintln(output, "Checking the dedicated TAP adapter...")
		return ensureTAP(ctx, target)
	}
	stop := func() error {
		if err := waitInstalledProcesses(target, "nlroom.exe", true); err != nil {
			return err
		}
		return stopInstalledService(ctx, target)
	}
	if o.action == "remove" {
		return removeWindowsBundle(ctx, target, o.purge, stop)
	}
	fmt.Fprintln(output, "Verifying the package and existing installation...")
	if strings.EqualFold(o.source, target) || strings.HasPrefix(strings.ToLower(o.source), strings.ToLower(target)+string(os.PathSeparator)) {
		return errors.New("run the complete installer from outside the installation directory")
	}
	if err := checkWindowsTree(o.source, false); err != nil {
		return err
	}
	b, err := readBundle(o.source)
	if err != nil {
		return err
	}
	if b.version != model.ClientVersion {
		return errors.New("installer and payload versions differ")
	}
	var processMachine, nativeMachine uint16
	if err := windows.IsWow64Process2(windows.CurrentProcess(), &processMachine, &nativeMachine); err != nil {
		return err
	}
	if (nativeMachine == pe.IMAGE_FILE_MACHINE_AMD64 && b.arch != "amd64") || (nativeMachine == pe.IMAGE_FILE_MACHINE_ARM64 && b.arch != "arm64") || b.arch != runtime.GOARCH {
		return errors.New("use the installer matching native Windows architecture")
	}
	// A partial installation must remain repairable. Only readable version
	// metadata is needed for the downgrade guard, never the old file hashes.
	oldBuild, err := os.ReadFile(filepath.Join(target, "BUILD.txt"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if v := bundleVersion.FindSubmatch(oldBuild); len(v) == 2 && model.CompareVersion(b.version, string(v[1])) < 0 {
		return errors.New("installing an older version is not supported")
	}
	if !b.gui {
		for _, name := range []string{"nlroom.exe", "Uninstall.exe"} {
			if _, err := os.Stat(filepath.Join(target, name)); err == nil {
				return errors.New("use the complete desktop installer")
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	s, err := openInstalledService(target)
	if err != nil {
		return err
	}
	if s != nil {
		s.Close()
		if readErr != nil {
			return errors.New("installed service has no verifiable owner")
		}
	}
	fmt.Fprintln(output, "Stopping the client and network service...")
	if err := stop(); err != nil {
		return err
	}
	if err := prepareBundleDirectories(target, b); err != nil {
		return err
	}
	fmt.Fprintln(output, "Replacing program files...")
	if err := overwriteBundle(o.source, target, b, createBundleTemp, protectBundlePath); err != nil {
		return fmt.Errorf("%w; run the complete installer again to repair", err)
	}
	if err := removeLegacyRecovery(target); err != nil {
		return err
	}
	fmt.Fprintln(output, "Starting and checking the network service...")
	if err := activateBundle(ctx, target, o.owner, b.version); err != nil {
		return fmt.Errorf("%w; run the complete installer again to repair", err)
	}
	fmt.Fprintln(output, "NodeLane Room installed. Open the client as the installation user.")
	return nil
}

func removeWindowsBundle(ctx context.Context, target string, purge bool, stop func() error) error {
	if err := stop(); err != nil {
		return err
	}
	if err := removeTAP(target); err != nil {
		return err
	}
	s, err := openInstalledService(target)
	if err != nil {
		return err
	}
	if s != nil {
		err = s.Delete()
		s.Close()
		if err != nil {
			return err
		}
	}
	if err := removeLegacyRecovery(target); err != nil {
		return err
	}
	for _, name := range []string{"update-job.bin"} {
		if err := os.Remove(filepath.Join(platform.DefaultDir(), name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := removeAutostart(target); err != nil {
		return err
	}
	// The guard file remains open until completion; remove identity files without
	// deleting the locked directory. Reinstallation reuses its protected binding.
	if purge {
		entries, err := os.ReadDir(platform.DefaultDir())
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Name() == "update.lock" || entry.Name() == "install.lock" {
				continue
			}
			path := filepath.Join(platform.DefaultDir(), entry.Name())
			if err := checkWindowsTree(path, true); err != nil {
				return err
			}
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
	}
	if err := checkWindowsTree(target, true); err != nil {
		return err
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return unregisterBundle()
}

// Only removes artifacts created by older installers. New installations never
// create backup directories, recovery journals or a boot recovery task.
func removeLegacyRecovery(target string) error {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	task := filepath.Join(system, "schtasks.exe")
	if hiddenCommand(task, "/Query", "/TN", "NodeLaneRoomUpdateRecovery").Run() == nil {
		if err := hiddenCommand(task, "/Delete", "/TN", "NodeLaneRoomUpdateRecovery", "/F").Run(); err != nil {
			return errors.New("could not remove legacy update task")
		}
	}
	// Every recursive target is a fixed sibling of the KnownFolder install path.
	for _, suffix := range []string{".previous", ".pending", ".install.json", ".install.json.tmp"} {
		path := target + suffix
		if err := checkWindowsTree(path, true); err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func removeAutostart(target string) error {
	data, err := os.ReadFile(filepath.Join(platform.DefaultDir(), "owner.sid"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	sid := strings.TrimSpace(string(data))
	if _, err := windows.StringToSid(sid); err != nil {
		return err
	}
	base := sid + `\Software\Microsoft\Windows\CurrentVersion`
	k, err := registry.OpenKey(registry.USERS, base+`\Run`, registry.QUERY_VALUE|registry.SET_VALUE)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		value, _, err := k.GetStringValue(name)
		if err != nil {
			continue
		}
		args, err := windows.DecomposeCommandLine(value)
		if err != nil || len(args) != 1 || !strings.EqualFold(args[0], filepath.Join(target, "nlroom.exe")) {
			continue
		}
		if err := k.DeleteValue(name); err != nil {
			return err
		}
		approved, err := registry.OpenKey(registry.USERS, base+`\Explorer\StartupApproved\Run`, registry.SET_VALUE)
		if err == nil {
			_ = approved.DeleteValue(name)
			approved.Close()
		}
	}
	return nil
}

func protectPrivatePath(path string) error {
	sd, err := windows.SecurityDescriptorFromString("O:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)")
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

func hiddenCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            windows.Handle
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance                          windows.Handle
	IDList                            uintptr
	Class                             *uint16
	ClassKey                          windows.Handle
	HotKey                            uint32
	Icon, Process                     windows.Handle
}

func shellExecute(file string, args []string, verb string) (windows.Handle, error) {
	file16, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return 0, err
	}
	args16, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return 0, err
	}
	verb16, err := windows.UTF16PtrFromString(verb)
	if err != nil {
		return 0, err
	}
	info := shellExecuteInfo{Size: uint32(unsafe.Sizeof(shellExecuteInfo{})), Mask: 0x40 | 0x100 | 0x400, File: file16, Parameters: args16, Verb: verb16, Show: 0}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		return 0, err
	}
	defer windows.CoUninitialize()
	result, _, callErr := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		return 0, callErr
	}
	if info.Process == 0 {
		return 0, errors.New("elevated process did not start")
	}
	return info.Process, nil
}

func elevateSetup(exe string, o setupOptions, output io.Writer) error {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	o.pipe = fmt.Sprintf("NodeLaneRoom-install-%x", random)
	l, err := winio.ListenPipe(`\\.\pipe\`+o.pipe, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + o.owner + ")"})
	if err != nil {
		return err
	}
	defer l.Close()
	args := []string{o.action, "--owner-sid", o.owner, "--log-pipe", o.pipe}
	if o.action == "setup" {
		args = append(args, "--source", o.source)
	}
	if o.purge {
		args = append(args, "--purge")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(output, io.LimitReader(conn, 1<<20))
	}()
	h, err := shellExecute(exe, args, "runas")
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	if _, err := windows.WaitForSingleObject(h, windows.INFINITE); err != nil {
		return err
	}
	l.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("native installer exited with code %d", code)
	}
	return nil
}
