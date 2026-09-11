// Package update verifies signed releases and coordinates protected update files.
package update

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/theupdateframework/go-tuf/v2/metadata"
	"github.com/theupdateframework/go-tuf/v2/metadata/config"
	"github.com/theupdateframework/go-tuf/v2/metadata/updater"
)

// Set at build time from an offline root.json. No download can establish trust.
var TrustedRootBase64 string

var metadataName = regexp.MustCompile(`^(?:[1-9][0-9]*\.)?(root|timestamp|snapshot|targets)\.json$`)
var targetName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,179}$`)

const MaxPackageSize int64 = 2 << 30

func TrustedRoot() ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(TrustedRootBase64)
	if err != nil || len(b) == 0 {
		return nil, model.Failure("local_update_trust_unconfigured")
	}
	return b, nil
}

type bundleFetcher map[string]json.RawMessage

func (b bundleFetcher) DownloadFile(address string, limit int64, _ time.Duration) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	name := path.Base(u.Path)
	data, ok := b[name]
	if !ok {
		// Consistent snapshots request version-prefixed metadata.
		for _, role := range []string{"snapshot", "targets"} {
			if path.Ext(name) == ".json" && len(name) > len(role)+6 && name[len(name)-len(role)-5:] == role+".json" {
				data, ok = b[role+".json"]
			}
		}
	}
	if !ok {
		return nil, &metadata.ErrDownloadHTTP{StatusCode: 404, URL: address}
	}
	if int64(len(data)) > limit {
		return nil, model.Failure("local_update_metadata_invalid")
	}
	return data, nil
}

// VerifyRepository uses the complete TUF workflow, including expiration and
// rollback checks. cache is private and persists the highest trusted versions.
func VerifyRepository(root []byte, bundle map[string]json.RawMessage, cache string) (*updater.Updater, error) {
	if len(bundle) > 80 {
		return nil, model.Failure("local_update_metadata_invalid")
	}
	total := 0
	for name, b := range bundle {
		if !metadataName.MatchString(name) || !json.Valid(b) {
			return nil, model.Failure("local_update_metadata_invalid")
		}
		total += len(b)
	}
	if total > 2<<20 {
		return nil, model.Failure("local_update_metadata_invalid")
	}
	if cache != "" {
		if b, err := os.ReadFile(filepath.Join(cache, "root.json")); err == nil {
			root = b
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	cfg, err := config.New("https://metadata.invalid/", root)
	if err != nil {
		return nil, err
	}
	cfg.Fetcher = bundleFetcher(bundle)
	cfg.MaxRootRotations = 64
	cfg.LocalMetadataDir, cfg.LocalTargetsDir = cache, cache
	cfg.DisableLocalCache = cache == ""
	u, err := updater.New(cfg)
	if err != nil {
		return nil, model.Failure("local_update_metadata_invalid")
	}
	if err = u.Refresh(); err != nil {
		return nil, fmt.Errorf("update_metadata_invalid: %w", err)
	}
	return u, nil
}

func Artifact(u *updater.Updater, target string) (model.UpdateArtifact, error) {
	var a model.UpdateArtifact
	if !targetName.MatchString(target) {
		return a, model.Failure("local_update_package_invalid")
	}
	t, err := u.GetTargetInfo(target)
	if err != nil {
		return a, err
	}
	if t.Custom == nil || json.Unmarshal(*t.Custom, &a) != nil || !model.ValidVersion(a.Version) || !model.ValidUpdatePlatform(a.OS, a.Arch) {
		return a, model.Failure("local_update_package_invalid")
	}
	if (a.OS == "windows" && path.Ext(target) != ".exe") || (a.OS == "linux" && path.Ext(target) != ".deb") || len(t.Hashes["sha256"]) != 32 || t.Length <= 0 || t.Length > MaxPackageSize {
		return a, model.Failure("local_update_package_invalid")
	}
	a.Target, a.Size, a.SHA256 = target, t.Length, hex.EncodeToString(t.Hashes["sha256"])
	return a, nil
}

// VerifyAdvance seeds a disposable cache from the last committed repository;
// rejected uploads cannot advance the shared database's trusted metadata.
func VerifyAdvance(root []byte, old, next map[string]json.RawMessage) (*updater.Updater, error) {
	dir, err := os.MkdirTemp("", "nlroom-tuf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	for name, b := range old {
		if metadataName.MatchString(name) {
			if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
				return nil, err
			}
		}
	}
	u, err := VerifyRepository(root, next, dir)
	if err != nil {
		return nil, err
	}
	// Every supplied unversioned root must be the actual final trusted root.
	trusted := u.GetTrustedMetadataSet()
	actual, err := trusted.Root.ToBytes(false)
	if err != nil {
		return nil, err
	}
	var x, y any
	if json.Unmarshal(actual, &x) != nil || json.Unmarshal(next["root.json"], &y) != nil {
		return nil, model.Failure("local_update_trust_unconfigured")
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	if !bytes.Equal(xb, yb) {
		return nil, model.Failure("local_update_metadata_invalid")
	}
	return u, nil
}
