//go:build !linux

package agent

import "github.com/nodelane/nodelane-room/internal/model"

type udpTraffic struct{}

func startUDPTraffic(int) *udpTraffic            { return nil }
func (*udpTraffic) Close()                       {}
func (*udpTraffic) Sample() *model.TrafficSample { return nil }
