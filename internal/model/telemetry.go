package model

import "time"

const TelemetryRetention = 60 * time.Second

// NetworkSample contains observations, never credentials or discovery candidates.
type NetworkSample struct {
	Epoch       string         `json:"epoch"`
	At          time.Time      `json:"at"`
	Generation  uint64         `json:"generation"`
	Connections int            `json:"connections"`
	Peers       []LinkSample   `json:"peers"`
	Traffic     *TrafficSample `json:"traffic,omitempty"`
}

type TrafficSample struct {
	UploadBytes   uint64 `json:"upload_bytes"`
	DownloadBytes uint64 `json:"download_bytes"`
	Scope         string `json:"scope"`
}

type LinkSample struct {
	DeviceID    string    `json:"device_id"`
	IP          string    `json:"ip"`
	Mode        string    `json:"mode"`
	Remote      string    `json:"remote,omitempty"`
	RelayIPs    []string  `json:"relay_ips"`
	RTTMillis   *float64  `json:"rtt_ms,omitempty"`
	LossPercent *float64  `json:"loss_percent,omitempty"`
	ProbeAt     time.Time `json:"probe_at"`
	Country     string    `json:"country,omitempty"`
	Region      string    `json:"region,omitempty"`
}

type TelemetrySeries struct {
	DeviceID string          `json:"device_id"`
	RoomID   string          `json:"room_id,omitempty"`
	NodeID   string          `json:"node_id,omitempty"`
	Samples  []NetworkSample `json:"samples"`
}

type TelemetrySnapshot struct {
	ServerTime       time.Time         `json:"server_time"`
	RetentionSeconds int               `json:"retention_seconds"`
	StaleSeconds     int               `json:"stale_seconds"`
	GeoIP            bool              `json:"geoip"`
	GeoIPProvider    string            `json:"geoip_provider,omitempty"`
	Series           []TelemetrySeries `json:"series"`
}
