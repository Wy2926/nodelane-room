package pki

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/slackhq/nebula/cert"
)

type Authority struct {
	Certificate cert.Certificate
	Key         []byte
	PEM         string
}

func Generate(network netip.Prefix) (*Authority, error) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	t := cert.TBSCertificate{Version: cert.Version2, Name: "NodeLane Room CA", IsCA: true, Networks: []netip.Prefix{network}, PublicKey: pub, Curve: cert.Curve_CURVE25519, NotBefore: time.Now().UTC().Add(-time.Minute), NotAfter: time.Now().UTC().AddDate(1, 0, 0)}
	c, err := t.Sign(nil, cert.Curve_CURVE25519, key)
	if err != nil {
		return nil, err
	}
	b, err := c.MarshalPEM()
	return &Authority{Certificate: c, Key: key, PEM: string(b)}, err
}

// Parse accepts exactly one self-signed CA and its matching signing key.
func Parse(b, k []byte) (*Authority, error) {
	c, rest, err := cert.UnmarshalCertificateFromPEM(b)
	if err != nil || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("invalid CA certificate PEM")
	}
	key, rest, curve, err := cert.UnmarshalSigningPrivateKeyFromPEM(k)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(rest)) != 0 || !c.IsCA() || curve != cert.Curve_CURVE25519 || c.Expired(time.Now()) || !c.CheckSignature(c.PublicKey()) {
		return nil, errors.New("invalid or expired NodeLane CA")
	}
	if err = c.VerifyPrivateKey(curve, key); err != nil {
		return nil, err
	}
	return &Authority{Certificate: c, Key: key, PEM: string(b)}, nil
}

func (a *Authority) SigningPEM() string {
	return string(cert.MarshalSigningPrivateKeyToPEM(cert.Curve_CURVE25519, a.Key))
}

// ValidatePool checks the same constraints used when signing actual player and node leases.
func (a *Authority) ValidatePool(network netip.Prefix) error {
	// Room groups contain unpredictable room IDs, so a fixed CA group allowlist
	// cannot authorize every room even if it happens to contain the probe group.
	if len(a.Certificate.Groups()) != 0 {
		return errors.New("CA must not restrict groups")
	}
	if time.Until(a.Certificate.NotAfter()) < 10*time.Minute {
		return errors.New("CA must remain valid for at least 10 minutes")
	}
	_, pub, err := TunnelKey()
	if err != nil {
		return err
	}
	for _, group := range []string{"room:validation", "infrastructure"} {
		if _, err = a.Sign("validation", network, []string{group}, pub, time.Now().Add(10*time.Minute)); err != nil {
			return errors.New("CA does not authorize the address pool and required groups")
		}
	}
	return nil
}
func (a *Authority) Sign(name string, network netip.Prefix, groups []string, pub []byte, until time.Time) (cert.Certificate, error) {
	if len(pub) != 32 {
		return nil, errors.New("X25519 public key must be 32 bytes")
	}
	remote, err := ecdh.X25519().NewPublicKey(pub)
	if err != nil {
		return nil, err
	}
	testKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if _, err = testKey.ECDH(remote); err != nil {
		return nil, fmt.Errorf("invalid X25519 key: %w", err)
	}
	if until.After(a.Certificate.NotAfter()) {
		until = a.Certificate.NotAfter()
	}
	if time.Until(until) < time.Minute {
		return nil, errors.New("CA expires too soon to issue a lease")
	}
	t := cert.TBSCertificate{Version: cert.Version2, Name: name, Networks: []netip.Prefix{network}, Groups: groups, NotBefore: time.Now().UTC().Add(-30 * time.Second), NotAfter: until.UTC().Truncate(time.Second), PublicKey: pub, Curve: cert.Curve_CURVE25519}
	return t.Sign(a.Certificate, cert.Curve_CURVE25519, a.Key)
}
func TunnelKey() (private, public []byte, err error) {
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return k.Bytes(), k.PublicKey().Bytes(), nil
}
func PrivatePEM(key []byte) string {
	return string(cert.MarshalPrivateKeyToPEM(cert.Curve_CURVE25519, key))
}
