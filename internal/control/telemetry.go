package control

import (
	"encoding/json"
	"math"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

type telemetryEntry struct {
	series model.TelemetrySeries
	sizes  []int
}
type telemetryMemory struct {
	mu      sync.Mutex
	entries map[string]telemetryEntry
	bytes   int
}

func validSample(in model.NetworkSample, now time.Time) bool {
	if len(in.Epoch) != 32 {
		return false
	}
	if in.At.Before(now.Add(-15*time.Second)) || in.At.After(now.Add(10*time.Second)) || len(in.Peers) > 128 || in.Connections < len(in.Peers) || in.Connections > 65536 {
		return false
	}
	if in.Traffic != nil && in.Traffic.Scope != "overlay" && in.Traffic.Scope != "nebula_udp" {
		return false
	}
	seen := map[string]bool{}
	for _, p := range in.Peers {
		ip, err := netip.ParseAddr(p.IP)
		if err != nil || !ip.Is4() || len(p.DeviceID) != 64 || seen[p.DeviceID] || len(p.RelayIPs) > 64 || p.Country != "" || p.Region != "" {
			return false
		}
		seen[p.DeviceID] = true
		if p.Mode != "direct" && p.Mode != "relay" && p.Mode != "unknown" {
			return false
		}
		if p.Remote != "" {
			a, e := netip.ParseAddrPort(p.Remote)
			if e != nil || a.Port() == 0 || p.Mode != "direct" {
				return false
			}
		}
		for _, relay := range p.RelayIPs {
			if _, e := netip.ParseAddr(relay); e != nil {
				return false
			}
		}
		if p.ProbeAt.After(in.At.Add(time.Second)) {
			return false
		}
		for _, value := range []*float64{p.RTTMillis, p.LossPercent} {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0) {
				return false
			}
		}
		if p.RTTMillis != nil && *p.RTTMillis > 10000 || p.LossPercent != nil && *p.LossPercent > 100 {
			return false
		}
		if (p.RTTMillis != nil || p.LossPercent != nil) && p.ProbeAt.Before(now.Add(-model.TelemetryRetention)) {
			return false
		}
	}
	return true
}

func (m *telemetryMemory) pruneLocked(now time.Time) {
	for id, e := range m.entries {
		n := 0
		for n < len(e.series.Samples) && e.series.Samples[n].At.Before(now.Add(-model.TelemetryRetention)) {
			m.bytes -= e.sizes[n]
			n++
		}
		if n == len(e.series.Samples) {
			delete(m.entries, id)
			continue
		}
		if n > 0 {
			e.series.Samples = append([]model.NetworkSample(nil), e.series.Samples[n:]...)
			e.sizes = append([]int(nil), e.sizes[n:]...)
			m.entries[id] = e
		}
	}
}
func (m *telemetryMemory) prune(now time.Time) { m.mu.Lock(); defer m.mu.Unlock(); m.pruneLocked(now) }

func (m *telemetryMemory) put(identity model.TelemetrySeries, sample model.NetworkSample, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(now)
	if m.entries == nil {
		m.entries = map[string]telemetryEntry{}
	}
	e, exists := m.entries[identity.DeviceID]
	if exists && (e.series.RoomID != identity.RoomID || e.series.NodeID != identity.NodeID) {
		for _, size := range e.sizes {
			m.bytes -= size
		}
		e = telemetryEntry{}
		exists = false
		delete(m.entries, identity.DeviceID)
	}
	if exists {
		last := e.series.Samples[len(e.series.Samples)-1]
		if !sample.At.After(last.At) {
			return nil
		}
		if sample.At.Sub(last.At) < 4*time.Second {
			return ErrRateLimited
		}
	} else {
		e.series = identity
	}
	b, err := json.Marshal(sample)
	if err != nil {
		return ErrInvalid
	}
	// Account conservatively for Go strings, slices and map overhead; never evict
	// accepted, unexpired history to admit a noisy source.
	size := len(b)*4 + 1024
	if m.bytes+size > 64<<20 || !exists && len(m.entries) >= 1024 {
		return ErrRateLimited
	}
	e.series.Samples = append(e.series.Samples, sample)
	e.sizes = append(e.sizes, size)
	m.bytes += size
	m.entries[identity.DeviceID] = e
	return nil
}

func (m *telemetryMemory) snapshot(now time.Time, geo *GeoIP) model.TelemetrySnapshot {
	m.mu.Lock()
	m.pruneLocked(now)
	out := model.TelemetrySnapshot{ServerTime: now, RetentionSeconds: 60, StaleSeconds: 15, GeoIP: geo.available(), Series: []model.TelemetrySeries{}}
	if geo != nil {
		out.GeoIPProvider = geo.provider()
	}
	for _, e := range m.entries {
		series := e.series
		series.Samples = append([]model.NetworkSample(nil), series.Samples...)
		for i := range series.Samples {
			series.Samples[i].Peers = append([]model.LinkSample(nil), series.Samples[i].Peers...)
		}
		out.Series = append(out.Series, series)
	}
	m.mu.Unlock()
	locations := map[string][2]string{}
	for i := range out.Series {
		for j := range out.Series[i].Samples {
			for k := range out.Series[i].Samples[j].Peers {
				p := &out.Series[i].Samples[j].Peers[k]
				if geo != nil {
					location, found := locations[p.Remote]
					if !found {
						location[0], location[1] = geo.lookup(p.Remote)
						locations[p.Remote] = location
					}
					p.Country, p.Region = location[0], location[1]
				}
			}
		}
	}
	sort.Slice(out.Series, func(i, j int) bool { return out.Series[i].DeviceID < out.Series[j].DeviceID })
	return out
}
