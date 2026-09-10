package pki

import (
	"net/netip"
	"testing"
	"time"

	"github.com/slackhq/nebula/cert"
)

func TestUploadedCAConstraints(t *testing.T) {
	pool := netip.MustParsePrefix("10.203.0.0/16")
	a, err := Generate(pool)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		groups   []string
		network  netip.Prefix
		validity time.Duration
		wantOK   bool
	}{
		{"unrestricted", nil, pool, time.Hour, true},
		{"fixed room groups", []string{"room:validation", "infrastructure"}, pool, time.Hour, false},
		{"wrong network", nil, netip.MustParsePrefix("10.204.0.0/16"), time.Hour, false},
		{"expires too soon", nil, pool, time.Minute, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tbs := cert.TBSCertificate{Version: cert.Version2, Name: "Upload test", IsCA: true, Networks: []netip.Prefix{tc.network}, Groups: tc.groups, PublicKey: a.Certificate.PublicKey(), Curve: cert.Curve_CURVE25519, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(tc.validity)}
			c, err := tbs.Sign(nil, cert.Curve_CURVE25519, a.Key)
			if err != nil {
				t.Fatal(err)
			}
			pem, err := c.MarshalPEM()
			if err != nil {
				t.Fatal(err)
			}
			uploaded, err := Parse(pem, []byte(a.SigningPEM()))
			if err != nil {
				t.Fatal(err)
			}
			if err = uploaded.ValidatePool(pool); (err == nil) != tc.wantOK {
				t.Fatalf("CA acceptance = %v, want %v", err == nil, tc.wantOK)
			}
			if _, err = Parse(append(pem, pem...), []byte(a.SigningPEM())); err == nil {
				t.Fatal("multiple certificates accepted")
			}
		})
	}
}
