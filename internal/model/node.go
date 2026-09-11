package model

import "time"

type NodeConfig struct {
	Name       string `json:"name"`
	Region     string `json:"region"`
	Address    string `json:"address"`
	Lighthouse bool   `json:"lighthouse"`
	Relay      bool   `json:"relay"`
	Notes      string `json:"notes"`
}

type EnrollmentChallengeRequest struct {
	Key string `json:"key"`
	ChallengeRequest
}

type NodeOperation struct {
	ID         string    `json:"id"`
	NodeID     string    `json:"node_id"`
	Generation int64     `json:"generation"`
	Revision   int64     `json:"revision"`
	Action     string    `json:"action"`
	State      string    `json:"state"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type OperationResult struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

type NodeReport struct {
	Version         string      `json:"version"`
	Engine          string      `json:"engine"`
	Error           string      `json:"error,omitempty"`
	AppliedRevision int64       `json:"applied_revision"`
	AppliedConfig   *NodeConfig `json:"applied_config,omitempty"`
	LeaseExpiresAt  time.Time   `json:"lease_expires_at"`
	LastRenewal     time.Time   `json:"last_renewal"`
	Probes          []NodeProbe `json:"probes"`
}

type NodeProbe struct {
	DeviceID    string    `json:"device_id"`
	Address     string    `json:"address"`
	Remote      string    `json:"remote,omitempty"`
	Mode        string    `json:"mode"`
	RTTMillis   *float64  `json:"rtt_ms,omitempty"`
	LossPercent *float64  `json:"loss_percent,omitempty"`
	At          time.Time `json:"at"`
}

type NodeSyncRequest struct {
	Generation int64             `json:"generation"`
	Report     NodeReport        `json:"report"`
	Results    []OperationResult `json:"results"`
}

type NodeSync struct {
	Node       Node            `json:"node"`
	Snapshot   Snapshot        `json:"snapshot"`
	Operations []NodeOperation `json:"operations"`
}

type NodeLocalStatus struct {
	Status
	Node       *Node      `json:"node,omitempty"`
	Report     NodeReport `json:"report"`
	Server     string     `json:"server"`
	Registered bool       `json:"registered"`
	Version    string     `json:"version"`
}

func (n Node) Config() NodeConfig {
	return NodeConfig{Name: n.Name, Region: n.Region, Address: n.Address, Lighthouse: n.Lighthouse, Relay: n.Relay, Notes: n.Notes}
}
