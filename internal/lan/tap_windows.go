package lan

import (
	"encoding/binary"
	"github.com/nodelane/nodelane-room/internal/model"
	"net"
	"net/netip"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

// Open only the explicitly provisioned NodeLane TAP-Windows6 adapter. Runtime
// code never downloads/installs a driver or reuses another VPN's adapter.
func OpenTAP(name string, prefix netip.Prefix) (tap *TAP, failure error) {
	defer func() {
		if failure != nil && model.Code(failure) == "system_internal_error" {
			failure = model.Failure("local_tap_unavailable")
		}
	}()
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, model.Failure("local_tap_missing")
	}
	luid, err := winipcfg.LUIDFromIndex(uint32(iface.Index))
	if err != nil {
		return nil, err
	}
	guid, err := luid.GUID()
	if err != nil {
		return nil, err
	}
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Class\{4D36E972-E325-11CE-BFC1-08002BE10318}`, registry.READ)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	valid, err := registeredTAP(root, guid.String())
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, model.Failure("local_tap_invalid")
	}
	path, err := windows.UTF16PtrFromString(`\\.\Global\` + guid.String() + `.tap`)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_SYSTEM|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, err
	}
	mac := make(net.HardwareAddr, 6)
	if err = tapIOCTL(handle, 1, nil, mac); err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	mtu := make([]byte, 4)
	if err = tapIOCTL(handle, 3, nil, mtu); err != nil || binary.LittleEndian.Uint32(mtu) != 1500 {
		windows.CloseHandle(handle)
		return nil, model.Failure("local_tap_invalid")
	}
	if err = tapIOCTL(handle, 6, []byte{1, 0, 0, 0}, nil); err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	file, err := winio.NewOpenFile(handle)
	if err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	t := &TAP{ReadWriteCloser: file, MAC: mac, Name: name}
	t.cleanup = func() error { return luid.SetIPAddresses(nil) }
	t.Activate = func() error {
		for _, family := range []winipcfg.AddressFamily{windows.AF_INET, windows.AF_INET6} {
			row, e := luid.IPInterface(family)
			if e != nil {
				return e
			}
			row.NLMTU = 1500
			row.DadTransmits = 0
			row.ForwardingEnabled = false
			row.AdvertisingEnabled = false
			row.RouterDiscoveryBehavior = winipcfg.RouterDiscoveryDisabled
			row.DisableDefaultRoutes = true
			if e = row.Set(); e != nil {
				return e
			}
		}
		return luid.SetIPAddresses(tapAddresses(prefix, mac))
	}
	return t, nil
}

func registeredTAP(root registry.Key, guid string) (bool, error) {
	keys, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return false, err
	}
	for _, name := range keys {
		key, err := registry.OpenKey(root, name, registry.READ)
		if err != nil {
			continue
		}
		id, _, _ := key.GetStringValue("NetCfgInstanceId")
		component, _, _ := key.GetStringValue("ComponentId")
		key.Close()
		// Both IDs are declared by the signed TAP-Windows6 INF.
		if strings.EqualFold(id, guid) && (strings.EqualFold(component, "tap0901") || strings.EqualFold(component, `root\tap0901`)) {
			return true, nil
		}
	}
	return false, nil
}

// TAP-Windows6 public IOCTL ABI (tap-windows.h, MIT), with cancellable
// overlapped device I/O supplied by the existing go-winio dependency.
func tapIOCTL(handle windows.Handle, request uint32, in, out []byte) error {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(event)
	overlap := windows.Overlapped{HEvent: event}
	var input, output *byte
	if len(in) > 0 {
		input = &in[0]
	}
	if len(out) > 0 {
		output = &out[0]
	}
	var n uint32
	err = windows.DeviceIoControl(handle, 0x22<<16|request<<2, input, uint32(len(in)), output, uint32(len(out)), &n, &overlap)
	if err == windows.ERROR_IO_PENDING {
		err = windows.GetOverlappedResult(handle, &overlap, &n, true)
	}
	return err
}
