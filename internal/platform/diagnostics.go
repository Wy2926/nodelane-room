package platform

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
)

func Diagnostics() map[string]any {
	out := map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH}
	if runtime.GOOS == "windows" {
		exe, err := os.Executable()
		if err == nil {
			arch := runtime.GOARCH
			if arch == "386" {
				arch = "x86"
			}
			path := filepath.Join(filepath.Dir(exe), "dist", "windows", "wintun", "bin", arch, "wintun.dll")
			_, err = os.Stat(path)
			out["wintun_present"] = err == nil
			out["wintun_path"] = path
		}
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
