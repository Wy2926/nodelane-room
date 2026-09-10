package platform

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/nodelane/nodelane-room/internal/client"
)

func SaveIdentity(dir string, i client.Identity) error {
	if err := SecureDir(dir); err != nil {
		return err
	}
	b, err := json.Marshal(i)
	if err != nil {
		return err
	}
	b, err = protect(b)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".identity-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(dir, "identity.bin")); err != nil {
		return err
	}
	return SyncDir(dir)
}
func LoadIdentity(dir string) (client.Identity, error) {
	var i client.Identity
	b, err := os.ReadFile(filepath.Join(dir, "identity.bin"))
	if err != nil {
		return i, err
	}
	b, err = unprotect(b)
	if err != nil {
		return i, err
	}
	err = json.Unmarshal(b, &i)
	if err == nil && (i.ID() == "" || client.ValidateURL(i.Server) != nil) {
		err = errors.New("invalid persisted device identity")
	}
	return i, err
}
