package agent

import (
	"github.com/nodelane/nodelane-room/internal/model"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

func interfaceTraffic(name string) *model.TrafficSample {
	rows, err := winipcfg.GetIfTable2Ex(winipcfg.MibIfEntryNormal)
	if err != nil {
		return nil
	}
	for _, row := range rows {
		if row.Alias() == name {
			return &model.TrafficSample{UploadBytes: row.OutOctets, DownloadBytes: row.InOctets, Scope: "overlay"}
		}
	}
	return nil
}
