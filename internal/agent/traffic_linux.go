package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nodelane/nodelane-room/internal/model"
)

func interfaceTraffic(name string) *model.TrafficSample {
	if name == "" || filepath.Base(name) != name {
		return nil
	}
	read := func(counter string) (uint64, error) {
		b, err := os.ReadFile(filepath.Join("/sys/class/net", name, "statistics", counter))
		if err != nil {
			return 0, err
		}
		return strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	}
	rx, err := read("rx_bytes")
	if err != nil {
		return nil
	}
	tx, err := read("tx_bytes")
	if err != nil {
		return nil
	}
	// TUN RX is traffic injected by Nebula into the host; TX leaves the host.
	return &model.TrafficSample{UploadBytes: tx, DownloadBytes: rx, Scope: "overlay"}
}
