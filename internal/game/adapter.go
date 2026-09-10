package game

import (
	"github.com/nodelane/nodelane-room/internal/model"
	"time"
)

// Adapter is the product-facing contract; identity, transport and certificate
// policy remain outside game implementations. Events contain no credentials.
type Adapter interface {
	Update(model.Snapshot)
	Observations() []Observation
	Notice(ip, room, endpointID string)
	Events() <-chan DiscoveryEvent
	Close()
}

type DiscoveryEvent struct {
	Kind       string    `json:"kind"`
	EndpointID string    `json:"endpoint_id,omitempty"`
	DeviceID   string    `json:"device_id"`
	Protocol   string    `json:"protocol"`
	Port       uint16    `json:"port"`
	MOTD       string    `json:"motd,omitempty"`
	At         time.Time `json:"at"`
}

var _ Adapter = (*Minecraft)(nil)

func (m *Minecraft) Events() <-chan DiscoveryEvent { return m.events }

// Events are bounded hints. Consumers can recover authoritative state via
// observations/control snapshots if they do not drain the channel quickly.
func (m *Minecraft) emit(e DiscoveryEvent) {
	if m.closed {
		return
	}
	e.At = time.Now().UTC()
	select {
	case m.events <- e:
	default:
	}
}
