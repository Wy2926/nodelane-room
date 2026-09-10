package pki

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
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
func Load(certPath, keyPath string) (*Authority, error) {
	b, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	c, _, err := cert.UnmarshalCertificateFromPEM(b)
	if err != nil {
		return nil, err
	}
	k, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	key, _, curve, err := cert.UnmarshalSigningPrivateKeyFromPEM(k)
	if err != nil {
		return nil, err
	}
	if !c.IsCA() || curve != cert.Curve_CURVE25519 || c.Expired(time.Now()) || !c.CheckSignature(c.PublicKey()) {
		return nil, errors.New("invalid or expired NodeLane CA")
	}
	if err = c.VerifyPrivateKey(curve, key); err != nil {
		return nil, err
	}
	return &Authority{Certificate: c, Key: key, PEM: string(b)}, nil
}
func (a *Authority) Save(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, f := range []struct {
		name string
		data []byte
	}{{"ca.key", cert.MarshalSigningPrivateKeyToPEM(cert.Curve_CURVE25519, a.Key)}, {"ca.crt", []byte(a.PEM)}} {
		p := filepath.Join(dir, f.name)
		fd, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = fd.Write(f.data)
		ce := fd.Close()
		if err != nil {
			return err
		}
		if ce != nil {
			return ce
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
