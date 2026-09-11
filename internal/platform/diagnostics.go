package platform

import (
	"net"
	"os"
	"runtime"
)

func Diagnostics() map[string]any {
	out := map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH}
	if runtime.GOOS == "windows" {
		_, err := net.InterfaceByName("nodelane0-lan")
		out["tap_interface_present"] = err == nil
	} else {
		_, err := os.Stat("/dev/net/tun")
		out["tun_device_present"] = err == nil
	}
	ifs, err := net.Interfaces()
	if err != nil {
		out["interface_error"] = err.Error()
		return out
	}
	interfaces := []map[string]any{}
	for _, i := range ifs {
		addrs, _ := i.Addrs()
		values := []string{}
		for _, a := range addrs {
			values = append(values, a.String())
		}
		interfaces = append(interfaces, map[string]any{"name": i.Name, "up": i.Flags&net.FlagUp != 0, "mtu": i.MTU, "addresses": values})
	}
	out["interfaces"] = interfaces
	return out
}
