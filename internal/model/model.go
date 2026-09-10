package model

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func ValidLabel(s string, maxBytes int) bool {
	if !utf8.ValidString(s) || strings.TrimSpace(s) == "" || len(s) > maxBytes {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

const (
	NebulaVersion = "v1.11.1+nodelane.d929786cba7f"
	DefaultPool   = "10.203.0.0/16"
	ProbePort     = 4243
	RoomCapacity  = 32
	LeaseDuration = 10 * time.Minute
)

type Device struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PublicKey []byte `json:"public_key"`
}
type Room struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OwnerID   string    `json:"owner_id"`
	Game      string    `json:"game"`
	GameName  string    `json:"game_name"`
	Revision  int64     `json:"revision"`
	Capacity  int       `json:"capacity"`
	ExpiresAt time.Time `json:"expires_at"`
	Closed    bool      `json:"closed"`
}
type Member struct {
	DeviceID string    `json:"device_id"`
	Name     string    `json:"name"`
	IP       string    `json:"ip"`
	LastSeen time.Time `json:"last_seen"`
}
type Endpoint struct {
	ID        string    `json:"id"`
	DeviceID  string    `json:"device_id"`
	Protocol  string    `json:"protocol"`
	Port      uint16    `json:"port"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Node struct {
	ID         string     `json:"id"`
	DeviceID   string     `json:"device_id"`
	Generation int64      `json:"generation"`
	Revision   int64      `json:"revision"`
	State      string     `json:"state"`
	Notes      string     `json:"notes"`
	Report     NodeReport `json:"report"`
	Name       string     `json:"name"`
	Region     string     `json:"region"`
	IP         string     `json:"ip"`
	Address    string     `json:"address"`
	Lighthouse bool       `json:"lighthouse"`
	Relay      bool       `json:"relay"`
	Draining   bool       `json:"draining"`
	LastSeen   time.Time  `json:"last_seen"`
}
type Snapshot struct {
	Room       *Room      `json:"room,omitempty"`
	Game       *Game      `json:"game,omitempty"`
	Members    []Member   `json:"members"`
	Endpoints  []Endpoint `json:"endpoints"`
	Nodes      []Node     `json:"nodes"`
	Blocklist  []string   `json:"blocklist"`
	ServerTime time.Time  `json:"server_time"`
}
type Lease struct {
	IP          string    `json:"ip"`
	Network     string    `json:"network"`
	CA          string    `json:"ca"`
	Certificate string    `json:"certificate"`
	Fingerprint string    `json:"fingerprint"`
	ExpiresAt   time.Time `json:"expires_at"`
	RoomID      string    `json:"room_id,omitempty"`
	Node        *Node     `json:"node,omitempty"`
}
type ChallengeRequest struct {
	DeviceID  string `json:"device_id"`
	Name      string `json:"name"`
	PublicKey []byte `json:"public_key"`
}
type Challenge struct {
	ID    string `json:"id"`
	Nonce []byte `json:"nonce"`
}
type VerifyRequest struct {
	ID        string `json:"id"`
	Signature []byte `json:"signature"`
}
type Session struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	DeviceID  string    `json:"device_id"`
}
type RoomRequest struct {
	Name string `json:"name"`
	Game string `json:"game"`
}
type JoinRequest struct {
	Code string `json:"code"`
}
type MemberRequest struct {
	DeviceID string `json:"device_id"`
}
type LeaseRequest struct {
	Revision  int64  `json:"revision,omitempty"`
	PublicKey []byte `json:"public_key"`
}
type Invitation struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}
type RoomResult struct {
	Room       Room        `json:"room"`
	Invitation *Invitation `json:"invitation,omitempty"`
}

type EndpointRequest struct {
	Protocol string `json:"protocol"`
	Port     uint16 `json:"port"`
}
type Peer struct {
	DeviceID    string   `json:"device_id"`
	Name        string   `json:"name"`
	IP          string   `json:"ip"`
	Mode        string   `json:"mode"`
	RTTMillis   *float64 `json:"rtt_ms,omitempty"`
	LossPercent *float64 `json:"loss_percent,omitempty"`
	Error       string   `json:"error,omitempty"`
}
type Status struct {
	Version         string            `json:"version"`
	ProtocolVersion int               `json:"protocol_version"`
	Server          string            `json:"server"`
	Name            string            `json:"name"`
	SelectedRoom    string            `json:"selected_room"`
	Ports           []EndpointRequest `json:"ports"`
	Game            *Game             `json:"game,omitempty"`
	Members         []Member          `json:"members"`
	Endpoints       []Endpoint        `json:"endpoints"`
	SnapshotAt      time.Time         `json:"snapshot_at"`
	DeviceID        string            `json:"device_id"`
	Control         string            `json:"control"`
	Engine          string            `json:"engine"`
	Error           string            `json:"error,omitempty"`
	Room            *Room             `json:"room,omitempty"`
	LeaseExpiresAt  time.Time         `json:"lease_expires_at,omitempty"`
	IP              string            `json:"ip,omitempty"`
	Peers           []Peer            `json:"peers"`
}
