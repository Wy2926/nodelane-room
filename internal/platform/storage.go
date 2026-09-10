package platform

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/nodelane/nodelane-room/internal/client"
)

// SavePrivateFile publishes complete, protected data. Exclusive publication uses
// a hard link so concurrent requests cannot replace an instance's database locator.
func SavePrivateFile(path string, data []byte, replace bool) error {
	dir := filepath.Dir(path)
	if err := SecureDir(dir); err != nil {
		return err
	}
	b, err := protect(data)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".private-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if replace {
		err = os.Rename(f.Name(), path)
	} else {
		err = os.Link(f.Name(), path)
	}
	if err != nil {
		return err
	}
	return SyncDir(dir)
}

func LoadPrivateFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return unprotect(b)
}

func SaveIdentity(dir string, i client.Identity) error {
	b, err := json.Marshal(i)
	if err != nil {
		return err
	}
	return SavePrivateFile(filepath.Join(dir, "identity.bin"), b, true)
}
func LoadIdentity(dir string) (client.Identity, error) {
	var i client.Identity
	b, err := LoadPrivateFile(filepath.Join(dir, "identity.bin"))
	if err != nil {
		return i, err
	}
	err = json.Unmarshal(b, &i)
	if err == nil && (i.ID() == "" || client.ValidateURL(i.Server) != nil) {
		err = errors.New("invalid persisted device identity")
	}
	return i, err
}
