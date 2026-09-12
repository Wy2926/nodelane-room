package update

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func TestNativeTAPRegistration(t *testing.T) {
	path := `Software\NodeLaneRoom-Install-test-` + rand.Text()
	root, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := registry.CreateKey(root, "0000", registry.ALL_ACCESS)
	if err != nil {
		root.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		key.Close()
		_ = registry.DeleteKey(root, "0000")
		root.Close()
		if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
			t.Error(err)
		}
	})
	const guid = "{a986c01f-4256-4921-bce7-d3d3fb61898b}"
	for _, tc := range []struct {
		name, id, component, version string
		valid                        bool
	}{
		{"official", guid, `root\tap0901`, "9.27.0.0", true},
		{"legacy", strings.ToUpper(guid), "tap0901", "9.27.0.0", true},
		{"future", guid, "tap0901", "10.0.0.0", true},
		{"foreign-guid", "{9ea44b15-df8e-4211-b5e5-80543d159a94}", "tap0901", "9.27.0.0", false},
		{"foreign-driver", guid, "wintun", "9.27.0.0", false},
		{"old-driver", guid, "tap0901", "9.24.7.0", false},
		{"bad-version", guid, "tap0901", "9.27.0.0bad", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for name, value := range map[string]string{"NetCfgInstanceId": tc.id, "ComponentId": tc.component, "DriverVersion": tc.version} {
				if err := key.SetStringValue(name, value); err != nil {
					t.Fatal(err)
				}
			}
			found, err := validateTAPRegistration(key, guid)
			if (found && err == nil) != tc.valid {
				t.Fatalf("unexpected adapter result: %v", err)
			}
			if found != strings.EqualFold(tc.id, guid) {
				t.Fatalf("incorrect registration presence: %v", found)
			}
		})
	}
	if err := key.DeleteValue("NetCfgInstanceId"); err != nil {
		t.Fatal(err)
	}
	if found, err := validateTAPRegistration(key, guid); found || err != nil {
		t.Fatalf("unconfigured device is not a registered adapter: %v, %v", found, err)
	}
	if _, err := validateTAPRegistration(registry.Key(0), guid); err == nil {
		t.Fatal("registry read failure was treated as a missing adapter")
	}
}

func TestNativeTAPDiscoveryWithoutHostChanges(t *testing.T) {
	// Exercise SetupAPI and real driver-key access as the current user. A random
	// GUID must not match any installed device; this never creates/opens a TAP.
	id, err := windows.GenerateGUID()
	if err != nil {
		t.Fatal(err)
	}
	if found, err := validatePresentTAP(id.String()); err != nil || found {
		t.Fatalf("unexpected live discovery result: %v, %v", found, err)
	}

	// Reproduce a leftover connection name whose PnP device has been removed.
	// It must allow normal provisioning, rather than abort startup as an
	// unverifiable driver. All writes stay in a temporary current-user key.
	path := `Software\NodeLaneRoom-TAP-discovery-test-` + rand.Text()
	root, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = registry.DeleteKey(root, id.String()+`\Connection`)
		_ = registry.DeleteKey(root, id.String())
		root.Close()
		if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
			t.Error(err)
		}
	})
	key, _, err := registry.CreateKey(root, id.String()+`\Connection`, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	err = key.SetStringValue("Name", "nodelane0-lan")
	key.Close()
	if err != nil {
		t.Fatal(err)
	}
	if guid, err := findInstalledTAP(root); err != nil || guid != "" {
		t.Fatalf("removed adapter blocked provisioning: %s, %v", guid, err)
	}
}

func TestNativeAuthenticodePublisher(t *testing.T) {
	if dir := os.Getenv("NODELANE_TEST_TAP_DIR"); dir != "" {
		for name, publisher := range map[string]string{"tap0901.sys": "Microsoft Corporation", "tap0901.cat": "Microsoft Corporation", "tapctl.exe": "OpenVPN Inc."} {
			t.Run(name, func(t *testing.T) {
				if err := verifyPublisher(filepath.Join(dir, name), publisher); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(system, "kernel32.dll")
	if err := verifyPublisher(file, "Microsoft Corporation"); err != nil {
		t.Fatal(err)
	}
	if verifyPublisher(file, "OpenVPN Inc.") == nil {
		t.Fatal("wrong publisher accepted")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	data[0] ^= 0xff
	tampered := filepath.Join(t.TempDir(), "tampered.dll")
	if err := os.WriteFile(tampered, data, 0600); err != nil {
		t.Fatal(err)
	}
	if verifyPublisher(tampered, "Microsoft Corporation") == nil {
		t.Fatal("tampered signature accepted")
	}
}

func TestNativeChildExitStatusWithoutElevation(t *testing.T) {
	if os.Getenv("NODELANE_INSTALL_CHILD") == "1" {
		os.Exit(7)
	}
	t.Setenv("NODELANE_INSTALL_CHILD", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	h, err := shellExecute(exe, []string{"-test.run=^TestNativeChildExitStatusWithoutElevation$"}, "open")
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	result, err := windows.WaitForSingleObject(h, 10000)
	if err != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatalf("child did not exit: %v %v", result, err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil || code != 7 {
		t.Fatalf("lost child failure: %d %v", code, err)
	}
}

func TestSetupArgumentsRejectPrivilegedActionsBeforeMutation(t *testing.T) {
	for _, args := range [][]string{{"setup", "--source", "x", "extra"}, {"tap", "--purge"}, {"remove", "--managed"}, {"tap", "--source", "x"}, {"setup", "--unknown"}, {"setup", "--rollback"}} {
		handled, err := SetupCommand(context.Background(), args)
		if !handled || err == nil {
			t.Fatalf("accepted invalid operation %v", args)
		}
	}
}

func TestNativeInstallationRejectsReparsePoint(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	link := filepath.Join(base, "link")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlink creation requires Windows developer mode or privilege")
	}
	if err := checkWindowsTree(link, false); err == nil {
		t.Fatal("reparse point accepted")
	}
}

func TestNativeShortcutInTemporaryDirectory(t *testing.T) {
	dir := t.TempDir()
	shortcut := filepath.Join(dir, "NodeLane Room.lnk")
	if err := writeShortcut(shortcut, dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(shortcut)
	if err != nil || len(data) < 76 || data[0] != 0x4c {
		t.Fatalf("invalid shell link: %v", err)
	}
}
