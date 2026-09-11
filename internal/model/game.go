package model

import "time"

// GameNetwork defines the mandatory room Ethernet policy.
// EthernetTypes contains additional non-IP EtherTypes; 0 permits IEEE 802.3
// length/LLC frames. IPv4, IPv6 and ARP always use the built-in validators.
type GameNetwork struct {
	Version       int      `json:"version"`
	Broadcast     bool     `json:"broadcast"`
	Multicast     bool     `json:"multicast"`
	EthernetTypes []uint16 `json:"ethernet_types"`
}

type HeartbeatRequest struct {
	LANVersion int    `json:"lan_version"`
	MAC        string `json:"mac,omitempty"`
}

type LANStatus struct {
	Version   int    `json:"version"`
	Interface string `json:"interface"`
	MAC       string `json:"mac"`
	IPv6      string `json:"ipv6"`
	MTU       int    `json:"mtu"`
	Ready     bool   `json:"ready"`
}

// RoomManagement contains player-visible management data, never tunnel authority.
type RoomManagement struct {
	Room       Room      `json:"room"`
	Game       Game      `json:"game"`
	Members    []Member  `json:"members"`
	ServerTime time.Time `json:"server_time"`
}

// Game ports are compact authorized intervals. PortEnd=0 means a single port.
type GamePort struct {
	Protocol    string `json:"protocol"`
	Port        uint16 `json:"port"`
	PortEnd     uint16 `json:"port_end,omitempty"`
	Description string `json:"description,omitempty"`
}

type Game struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Summary       string      `json:"summary"`
	SourceURL     string      `json:"source_url"`
	CoverURL      string      `json:"cover_url"`
	BackgroundURL string      `json:"background_url"`
	Ports         []GamePort  `json:"ports"`
	Network       GameNetwork `json:"network"`
	Enabled       bool        `json:"enabled"`
	Revision      int64       `json:"revision"`
}

type GameUpdateRequest struct {
	Name     string      `json:"name"`
	Ports    []GamePort  `json:"ports"`
	Network  GameNetwork `json:"network"`
	Enabled  bool        `json:"enabled"`
	Revision int64       `json:"revision"`
}

type GameImportRequest struct {
	URL string `json:"url"`
}
