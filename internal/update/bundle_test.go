package update

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleManifestBoundaries(t *testing.T) {
	for _, name := range []string{"../nlroom.exe", "licenses/../../outside", "licenses/a:stream", "licenses\\escape", "/nlroom.exe", "licenses/NUL.txt", "licenses/sub./file", "drivers/tap/evil.exe", "setup.ps1", "licenses/./file"} {
		if bundleFile(name) {
			t.Errorf("accepted %q", name)
		}
	}
	for _, scenario := range []string{"valid", "modified", "missing", "case-duplicate", "wrong-machine", "unsigned-extra-command"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			pe := make([]byte, 96)
			copy(pe, "MZ")
			binary.LittleEndian.PutUint32(pe[60:], 64)
			copy(pe[64:], "PE\x00\x00")
			binary.LittleEndian.PutUint16(pe[68:], 0x8664)
			if scenario == "wrong-machine" {
				binary.LittleEndian.PutUint16(pe[68:], 0xaa64)
			}
			files := map[string][]byte{"nlroom-cli.exe": pe, "nlroom-service.exe": pe, "nlroom-update.exe": pe, "BUILD.txt": []byte("Component: client\nVersion: 0.3.0\nTarget: windows/amd64\n"), "THIRD_PARTY_NOTICES.txt": []byte("licenses")}
			var manifest strings.Builder
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
					t.Fatal(err)
				}
				if name == "nlroom-cli.exe" && scenario == "missing" {
					continue
				}
				digest := sha256.Sum256(data)
				manifest.WriteString(hex.EncodeToString(digest[:]) + "  " + name + "\n")
				if name == "nlroom-cli.exe" && scenario == "case-duplicate" {
					manifest.WriteString(hex.EncodeToString(digest[:]) + "  NLROOM-CLI.EXE\n")
				}
			}
			if scenario == "modified" {
				if err := os.WriteFile(filepath.Join(dir, "nlroom-cli.exe"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "unsigned-extra-command" {
				manifest.WriteString(strings.Repeat("0", 64) + "  install.ps1\n")
			}
			if err := os.WriteFile(filepath.Join(dir, "PAYLOAD.sha256"), []byte(manifest.String()), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := readBundle(dir)
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("unexpected verification result: %v", err)
			}
		})
	}
}

func TestBundleInterruptedOverwriteCanBeRepaired(t *testing.T) {
	for _, damaged := range []string{"missing-program", "corrupt-manifest", "interrupted-copy"} {
		t.Run(damaged, func(t *testing.T) {
			base := t.TempDir()
			source, target, state := filepath.Join(base, "source"), filepath.Join(base, "NodeLaneRoom"), filepath.Join(base, "state")
			for _, dir := range []string{source, target, state} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			payload := make([]byte, 96)
			copy(payload, "MZ")
			binary.LittleEndian.PutUint32(payload[60:], 64)
			copy(payload[64:], "PE\x00\x00")
			binary.LittleEndian.PutUint16(payload[68:], 0x8664)
			files := map[string][]byte{"nlroom-cli.exe": payload, "nlroom-service.exe": payload, "nlroom-update.exe": payload,
				"BUILD.txt": []byte("Version: 0.3.0\nTarget: windows/amd64\n"), "THIRD_PARTY_NOTICES.txt": []byte("licenses")}
			var manifest strings.Builder
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(source, name), data, 0600); err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(data)
				manifest.WriteString(hex.EncodeToString(digest[:]) + "  " + name + "\n")
			}
			if err := os.WriteFile(filepath.Join(source, "PAYLOAD.sha256"), []byte(manifest.String()), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(state, "identity"), []byte("preserved"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, "setup.ps1"), []byte("obsolete"), 0600); err != nil {
				t.Fatal(err)
			}
			if damaged == "corrupt-manifest" {
				if err := os.WriteFile(filepath.Join(target, "PAYLOAD.sha256"), []byte("broken"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			b, err := readBundle(source)
			if err != nil {
				t.Fatal(err)
			}
			if damaged == "interrupted-copy" {
				count := 0
				err := overwriteBundle(source, target, b, testBundleTemp, func(string) error {
					count++
					if count == 3 {
						return errors.New("simulated disk failure")
					}
					return nil
				})
				if err == nil {
					t.Fatal("ignored copy failure")
				}
				// The target is left in place; no automatic restoration.
				data, err := os.ReadFile(filepath.Join(target, "BUILD.txt"))
				if err != nil || !strings.Contains(string(data), "0.3.0") {
					t.Fatal("partial installation was rolled back")
				}
			}
			if err := overwriteBundle(source, target, b, testBundleTemp, func(string) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if _, err := readBundle(target); err != nil {
				t.Fatalf("repair did not restore the full package: %v", err)
			}
			if _, err := os.Stat(filepath.Join(target, "setup.ps1")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("obsolete script retained")
			}
			for _, suffix := range []string{".previous", ".pending", ".install.json"} {
				if _, err := os.Stat(target + suffix); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("recovery artifacts created")
				}
			}
			identity, err := os.ReadFile(filepath.Join(state, "identity"))
			if err != nil || string(identity) != "preserved" {
				t.Fatal("identity changed")
			}
		})
	}
}

func TestBundleCopyRejectsChangedSource(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "source"), filepath.Join(dir, "target")
	for name, data := range map[string]string{from: "tampered", to: "installed"} {
		if err := os.WriteFile(name, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	digest := sha256.Sum256([]byte("verified"))
	if err := copyBundleFile(from, to, hex.EncodeToString(digest[:]), testBundleTemp, func(string) error { return nil }); err == nil {
		t.Fatal("accepted changed source")
	}
	got, err := os.ReadFile(to)
	if err != nil || string(got) != "installed" {
		t.Fatal("failed copy replaced installed file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatal("temporary file leaked")
	}
}

func testBundleTemp(dir string) (*os.File, error) {
	return os.CreateTemp(dir, ".install-*")
}
