package model

import (
	"time"
)

type Capabilities struct {
	Contract             string `json:"contract"`
	LANVersion           int    `json:"lan_version"`
	LocalProtocolVersion int    `json:"local_protocol_version"`
	OIDCEnabled          bool   `json:"oidc_enabled"`
	Ready                bool   `json:"ready"`
}

type MembershipSelf struct {
	RoomID     string     `json:"room_id,omitempty"`
	DeviceID   string     `json:"device_id,omitempty"`
	State      string     `json:"state"`
	Reason     string     `json:"reason,omitempty"`
	Revision   int64      `json:"revision"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
}

type Permissions struct {
	Manage bool `json:"manage"`
	Join   bool `json:"join"`
	Leave  bool `json:"leave"`
}

type AccountStatus struct {
	User       User            `json:"user"`
	Device     UserDevice      `json:"device"`
	Membership MembershipSelf  `json:"membership"`
	Occupancy  *MembershipSelf `json:"occupancy,omitempty"`
}

type RoomPage struct {
	Rooms     []Room `json:"rooms"`
	Truncated bool   `json:"truncated"`
}

type InviteInfo struct {
	Active    bool       `json:"active"`
	Revision  int64      `json:"revision"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type TakeoverRequest struct {
	RoomID           string `json:"room_id"`
	DeviceID         string `json:"device_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}

type Operation struct {
	ID          string    `json:"id"`
	State       string    `json:"state"`
	Action      string    `json:"action,omitempty"`
	Deadline    time.Time `json:"deadline"`
	Result      *Result   `json:"result,omitempty"`
	KnownCommit bool      `json:"known_commit"`
}

// Receipt is stored only in idempotency.response. It is never an authorization grant.
type Receipt struct {
	Method   string    `json:"method"`
	Path     string    `json:"path"`
	Deadline time.Time `json:"deadline"`
	Status   int       `json:"status"`
	Result   Result    `json:"result"`
}

type Issue struct {
	Scope      string     `json:"scope"`
	Code       string     `json:"code"`
	OccurredAt time.Time  `json:"occurred_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

type NetworkState struct {
	State               string `json:"state"`
	Reason              string `json:"reason,omitempty"`
	Generation          uint64 `json:"generation"`
	AppliedGameRevision int64  `json:"applied_game_revision"`
}

type Freshness struct {
	ObservedAt time.Time `json:"observed_at"`
	SnapshotAt time.Time `json:"snapshot_at"`
}

type SavedCommand struct {
	Operation
	Request     []byte `json:"request,omitempty"`
	RequestHash string `json:"request_hash"`
	DeviceID    string `json:"device_id"`
	Method      string `json:"method,omitempty"`
	Path        string `json:"path,omitempty"`
	Body        []byte `json:"body,omitempty"`
}
