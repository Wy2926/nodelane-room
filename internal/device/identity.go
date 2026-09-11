// Package device defines persistent device identity and control origin validation.
package device

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/nodelane/nodelane-room/internal/model"
)

type Identity struct {
	SignedOut     bool   `json:"signed_out,omitempty"`
	PendingGuest  bool   `json:"pending_guest,omitempty"`
	Server        string `json:"server"`
	Name          string `json:"name"`
	PrivateKey    []byte `json:"private_key"`
	RoomID        string `json:"room_id,omitempty"`
	NodeID        string `json:"node_id,omitempty"`
	Generation    int64  `json:"generation,omitempty"`
	Node          bool   `json:"node"`
	CAFingerprint string `json:"ca_fingerprint,omitempty"`
}

func NewIdentity(server, name string) (Identity, error) {
	if err := ValidateURL(server); err != nil {
		return Identity{}, model.Validation("server", "invalid_format")
	}
	if !model.ValidLabel(name, 80) {
		rule := "invalid_format"
		if strings.TrimSpace(name) == "" {
			rule = "required"
		} else if len(name) > 80 {
			rule = "too_long"
		}
		return Identity{}, model.Validation("name", rule)
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	return Identity{Server: strings.TrimRight(server, "/"), Name: name, PrivateKey: key}, err
}
func ValidateURL(server string) error {
	u, err := url.Parse(server)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return errors.New("server must be an HTTPS origin")
	}
	if u.Scheme == "https" {
		return nil
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback()) {
		return nil
	}
	return errors.New("HTTP is allowed only on loopback; use HTTPS for remote servers")
}
func (i Identity) ID() string {
	if len(i.PrivateKey) != ed25519.PrivateKeySize {
		return ""
	}
	p := ed25519.PrivateKey(i.PrivateKey).Public().(ed25519.PublicKey)
	h := sha256.Sum256(p)
	return hex.EncodeToString(h[:])
}
