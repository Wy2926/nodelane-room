package update

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/nodelane/nodelane-room/internal/platform"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const tapClass = `{4D36E972-E325-11CE-BFC1-08002BE10318}`

var tapDriverVersion = regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`)

// Query the dedicated adapter without opening its data handle or requiring UAC.
// Disabled adapters remain registered and must not cause duplicate creation.
func installedTAP() (string, error) {
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Network\`+tapClass, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return "", err
	}
	defer root.Close()
	return findInstalledTAP(root)
}

func findInstalledTAP(root registry.Key) (string, error) {
	names, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return "", err
	}
	guid := ""
	for _, name := range names {
		id, err := windows.GUIDFromString(name)
		if err != nil {
			continue
		}
		k, err := registry.OpenKey(root, name+`\Connection`, registry.QUERY_VALUE)
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			continue
		}
		if err != nil {
			return "", err
		}
		alias, _, readErr := k.GetStringValue("Name")
		k.Close()
		if readErr != nil && !errors.Is(readErr, syscall.ERROR_FILE_NOT_FOUND) {
			return "", readErr
		}
		if !strings.EqualFold(alias, "nodelane0-lan") {
			continue
		}
		present, err := validatePresentTAP(id.String())
		if err != nil {
			return "", err
		}
		// Connection names can outlive their PnP devices after removal.
		if !present {
			continue
		}
		if guid != "" {
			return "", errors.New("multiple NodeLane adapters found")
		}
		guid = id.String()
	}
	if guid != "" {
		if _, err := net.InterfaceByName("nodelane0-lan"); err != nil {
			return "", errors.New("nodelane0-lan is registered but unavailable; enable the adapter or restart Windows")
		}
	}
	return guid, nil
}

// Use the driver's PnP registry key, without assuming Control\Class\NNNN is
// its location. DIGCF_PRESENT includes disabled devices but excludes ghosts.
func validatePresentTAP(guid string) (bool, error) {
	class, err := windows.GUIDFromString(tapClass)
	if err != nil {
		return false, err
	}
	devices, err := windows.SetupDiGetClassDevsEx(&class, "", 0, windows.DIGCF_PRESENT, 0, "")
	if err != nil {
		return false, fmt.Errorf("enumerate network devices: %w", err)
	}
	defer devices.Close()
	for index := 0; ; index++ {
		device, err := devices.EnumDeviceInfo(index)
		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("read network device: %w", err)
		}
		handle, err := devices.OpenDevRegKey(device, windows.DICS_FLAG_GLOBAL, 0, windows.DIREG_DRV, registry.QUERY_VALUE)
		if errors.Is(err, windows.ERROR_KEY_DOES_NOT_EXIST) {
			// A device that has not acquired a driver cannot own a connection.
			continue
		}
		if err != nil {
			return false, fmt.Errorf("open network driver registration: %w", err)
		}
		key := registry.Key(handle)
		found, err := validateTAPRegistration(key, guid)
		key.Close()
		if found || err != nil {
			return found, err
		}
	}
}

func validateTAPRegistration(key registry.Key, guid string) (bool, error) {
	id, _, err := key.GetStringValue("NetCfgInstanceId")
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read network adapter identifier: %w", err)
	}
	if !strings.EqualFold(id, guid) {
		return false, nil
	}
	component, _, err := key.GetStringValue("ComponentId")
	if err != nil {
		return true, fmt.Errorf("read dedicated TAP driver identifier: %w", err)
	}
	if !strings.EqualFold(component, "tap0901") && !strings.EqualFold(component, `root\tap0901`) {
		return true, errors.New("nodelane0-lan belongs to another driver")
	}
	version, _, err := key.GetStringValue("DriverVersion")
	if err != nil {
		return true, fmt.Errorf("read dedicated TAP driver version: %w", err)
	}
	var major, minor, patch, build int
	if n, _ := fmt.Sscanf(version, "%d.%d.%d.%d", &major, &minor, &patch, &build); !tapDriverVersion.MatchString(version) || n != 4 || major < 9 || (major == 9 && minor < 27) {
		return true, errors.New("dedicated TAP driver must be 9.27.0 or newer")
	}
	return true, nil
}

func verifyTAPFiles(dir string, names []string) error {
	if err := checkWindowsTree(dir, true); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if err != nil {
		return err
	}
	hashes := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "  ", 2)
		if len(parts) != 2 || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(parts[0]) {
			return errors.New("invalid TAP manifest")
		}
		name := strings.ToLower(parts[1])
		if name != "oemvista.inf" && name != "tap0901.cat" && name != "tap0901.sys" && name != "tapctl.exe" {
			return errors.New("invalid TAP file")
		}
		if _, ok := hashes[name]; ok {
			return errors.New("duplicate TAP file")
		}
		hashes[name] = parts[0]
	}
	if len(hashes) != 4 {
		return errors.New("incomplete TAP manifest")
	}
	for _, name := range names {
		file := filepath.Join(dir, name)
		if err := checkFileDigest(file, hashes[strings.ToLower(name)]); err != nil {
			return err
		}
		publisher := "Microsoft Corporation"
		if name == "tapctl.exe" {
			publisher = "OpenVPN Inc."
		}
		if name != "OemVista.inf" {
			if err := verifyPublisher(file, publisher); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyPublisher(path, publisher string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	info := windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: name}
	data := windows.WinTrustData{Size: uint32(unsafe.Sizeof(windows.WinTrustData{})), UIChoice: windows.WTD_UI_NONE, UnionChoice: windows.WTD_CHOICE_FILE, StateAction: windows.WTD_STATEACTION_VERIFY, FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&info)}
	err = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	defer func() {
		data.StateAction = windows.WTD_STATEACTION_CLOSE
		_ = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	}()
	if err != nil {
		return fmt.Errorf("TAP publisher signature verification failed: %w", err)
	}
	trust := windows.NewLazySystemDLL("wintrust.dll")
	provider, _, _ := trust.NewProc("WTHelperProvDataFromStateData").Call(uintptr(data.StateData))
	if provider == 0 {
		return errors.New("missing verified publisher")
	}
	signer, _, _ := trust.NewProc("WTHelperGetProvSignerFromChain").Call(provider, 0, 0, 0)
	if signer == 0 {
		return errors.New("missing verified signer")
	}
	// Prefixes of CRYPT_PROVIDER_SGNR and CRYPT_PROVIDER_CERT, per wintrust.h.
	type providerCert struct {
		Size uint32
		Cert *windows.CertContext
	}
	type providerSigner struct {
		Size       uint32
		VerifiedAt windows.Filetime
		Count      uint32
		Certs      *providerCert
	}
	// The return register contains an OS-owned pointer, not a Go heap address.
	s := *(**providerSigner)(unsafe.Pointer(&signer))
	if s.Count == 0 || s.Certs == nil || s.Certs.Cert == nil {
		return errors.New("missing signer certificate")
	}
	oid := []byte("2.5.4.10\x00")
	size := windows.CertGetNameString(s.Certs.Cert, windows.CERT_NAME_ATTR_TYPE, 0, unsafe.Pointer(&oid[0]), nil, 0)
	if size == 0 || size > 4096 {
		return errors.New("invalid publisher organization")
	}
	buffer := make([]uint16, size)
	windows.CertGetNameString(s.Certs.Cert, windows.CERT_NAME_ATTR_TYPE, 0, unsafe.Pointer(&oid[0]), &buffer[0], size)
	if windows.UTF16ToString(buffer) != publisher {
		return errors.New("unexpected TAP publisher")
	}
	return nil
}

func ensureTAP(ctx context.Context, target string) (failure error) {
	// Reuse comes before inspecting any unused driver installation resources.
	if guid, err := installedTAP(); err != nil || guid != "" {
		return err
	}
	dir := filepath.Join(target, "drivers", "tap")
	if err := verifyTAPFiles(dir, []string{"OemVista.inf", "tap0901.cat", "tap0901.sys", "tapctl.exe"}); err != nil {
		return err
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	cmd := hiddenCommand(filepath.Join(system, "pnputil.exe"), "/add-driver", filepath.Join(dir, "OemVista.inf"))
	if err := cmd.Run(); err != nil {
		return errors.New("TAP driver staging failed or requires a restart")
	}
	output, err := hiddenCommand(filepath.Join(dir, "tapctl.exe"), "create", "--name", "nodelane0-lan", "--hwid", `root\tap0901`).Output()
	if err != nil {
		return errors.New("could not create dedicated TAP adapter")
	}
	id, err := windows.GUIDFromString(strings.TrimSpace(string(output)))
	if err != nil {
		return errors.New("TAP creation returned an invalid identifier")
	}
	guid := id.String()
	// Record ownership immediately, before waiting or starting any data-plane work.
	record := filepath.Join(platform.DefaultDir(), "tap.guid")
	if err := writePrivateText(record, guid); err != nil {
		cleanup := hiddenCommand(filepath.Join(dir, "tapctl.exe"), "delete", guid).Run()
		if cleanup != nil {
			return errors.New("TAP ownership could not be saved; created adapter needs manual removal")
		}
		return err
	}
	defer func() {
		if failure == nil {
			return
		}
		if err := hiddenCommand(filepath.Join(dir, "tapctl.exe"), "delete", guid).Run(); err != nil {
			failure = fmt.Errorf("%w; the recorded TAP adapter needs manual cleanup", failure)
			return
		}
		if err := os.Remove(record); err != nil {
			failure = fmt.Errorf("%w; could not clear TAP ownership record", failure)
		}
	}()
	var lastErr error
	for attempt := 0; attempt < 50; attempt++ {
		found, queryErr := installedTAP()
		if queryErr == nil && strings.EqualFold(found, guid) {
			return nil
		}
		lastErr = queryErr
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("TAP adapter verification failed: %w", lastErr)
	}
	return errors.New("TAP adapter is not ready; restart Windows and open the client again")
}

func writePrivateText(path, value string) error {
	tmp, err := createSetupTemp(filepath.Dir(path), "O:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.WriteString(value + "\n"); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := protectPrivatePath(tmp.Name()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func removeTAP(target string) error {
	record := filepath.Join(platform.DefaultDir(), "tap.guid")
	data, err := os.ReadFile(record)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	id, err := windows.GUIDFromString(strings.TrimSpace(string(data)))
	if err != nil {
		return errors.New("invalid recorded TAP identifier")
	}
	// Look up the recorded GUID first. A renamed adapter must not be mistaken
	// for a deleted one, and a replacement manually created by the user is kept.
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Network\`+tapClass+`\`+id.String()+`\Connection`, registry.QUERY_VALUE)
	if err != nil && !errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return err
	}
	if err == nil {
		alias, _, err := k.GetStringValue("Name")
		k.Close()
		if err != nil {
			return err
		}
		if !strings.EqualFold(alias, "nodelane0-lan") {
			return errors.New("recorded TAP adapter was renamed; removal refused")
		}
		present, err := validatePresentTAP(id.String())
		if err != nil {
			return err
		}
		if !present {
			return os.Remove(record)
		}
		dir := filepath.Join(target, "drivers", "tap")
		if err := verifyTAPFiles(dir, []string{"tapctl.exe"}); err != nil {
			return err
		}
		if err := hiddenCommand(filepath.Join(dir, "tapctl.exe"), "delete", id.String()).Run(); err != nil {
			return errors.New("could not remove dedicated TAP adapter")
		}
	}
	return os.Remove(record)
}
