package model

import "time"

// User identity is independent of device keys, administrative roles and paid entitlements.
type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

type UserDevice struct {
	DeviceID  string     `json:"device_id"`
	Name      string     `json:"name"`
	Revoked   bool       `json:"revoked"`
	LastSeen  time.Time  `json:"last_seen"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type UserDetail struct {
	User     User          `json:"user"`
	Devices  []UserDevice  `json:"devices"`
	Rooms    []Room        `json:"rooms"`
	Sessions []UserSession `json:"sessions"`
}

type UserSession struct {
	DeviceID  string    `json:"device_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type UserPage struct {
	Users []User `json:"users"`
	Next  string `json:"next"`
}

type UserAction struct {
	Action   string `json:"action"`
	DeviceID string `json:"device_id,omitempty"`
	Reason   string `json:"reason"`
}

type LoginStart struct {
	Signature []byte `json:"signature"`
	DeviceID  string `json:"device_id"`
	PublicKey []byte `json:"public_key"`
	Name      string `json:"name"`
	Proof     string `json:"proof"`
}

type LoginAttempt struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type LoginClaim struct {
	ID        string `json:"id"`
	Proof     string `json:"proof"`
	Signature []byte `json:"signature"`
}

type LoginResult struct {
	State   string   `json:"state"`
	Session *Session `json:"session,omitempty"`
}

type OIDCSettings struct {
	Issuer       string `json:"issuer"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	Enabled      bool   `json:"enabled"`
}
