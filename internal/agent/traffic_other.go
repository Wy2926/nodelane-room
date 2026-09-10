//go:build !linux && !windows

package agent

import "github.com/nodelane/nodelane-room/internal/model"

func interfaceTraffic(string) *model.TrafficSample { return nil }
