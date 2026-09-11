package lan

import (
	"crypto/rand"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestRegisteredTAP(t *testing.T) {
	// Exercise actual registry discovery in a temporary user key, without a device.
	path := `Software\NodeLaneRoom-TAP-test-` + rand.Text()
	root, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, name := range []string{"0000", "0001"} {
			if err := registry.DeleteKey(root, name); err != nil && err != registry.ErrNotExist {
				t.Error(err)
			}
		}
		root.Close()
		if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
			t.Error(err)
		}
	})
	setDriver := func(name, guid, component string) {
		t.Helper()
		key, _, err := registry.CreateKey(root, name, registry.SET_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		defer key.Close()
		for name, value := range map[string]string{"NetCfgInstanceId": guid, "ComponentId": component} {
			if err := key.SetStringValue(name, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	const owned = "{a986c01f-4256-4921-bce7-d3d3fb61898b}"
	const foreign = "{9ea44b15-df8e-4211-b5e5-80543d159a94}"
	setDriver("0000", foreign, `root\tap0901`)
	for _, tc := range []struct {
		name, guid, component string
		want                  bool
	}{
		{"root-enumerated", owned, `root\tap0901`, true},
		{"legacy-id", owned, "tap0901", true},
		{"case-insensitive", strings.ToUpper(owned), `ROOT\TAP0901`, true},
		{"foreign-guid", foreign, `root\tap0901`, false},
		{"foreign-driver", owned, "wintun", false},
		{"similar-id", owned, `root\tap09010`, false},
		{"missing-id", owned, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setDriver("0001", tc.guid, tc.component)
			got, err := registeredTAP(root, owned)
			if err != nil || got != tc.want {
				t.Fatalf("registeredTAP = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}
