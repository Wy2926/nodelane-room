//go:build windows

package platform

import (
	"bytes"
	"testing"
)

func TestDPAPIProtection(t *testing.T) {
	plain := []byte("test identity secret")
	sealed, err := protect(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, plain) {
		t.Fatal("plaintext stored")
	}
	opened, err := unprotect(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(opened, plain) {
		t.Fatal("DPAPI roundtrip changed identity")
	}
	sealed[len(sealed)/2] ^= 0xff
	if _, err = unprotect(sealed); err == nil {
		t.Fatal("tampered DPAPI blob accepted")
	}
}

func TestStateDirectoryRejectsUserControlledParent(t *testing.T) {
	if err := SecureDir(t.TempDir()); err == nil {
		t.Fatal("accepted service state beneath a user-controlled directory")
	}
}
