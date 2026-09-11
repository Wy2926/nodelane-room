// Prepare signed update repositories offline. This tool never uploads or publishes.
package main

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
	"github.com/nodelane/nodelane-room/internal/update"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/theupdateframework/go-tuf/v2/metadata"
)

func main() {
	mode := flag.String("mode", "sign", "init, sign (also renews metadata), or rotate-root")
	keys := flag.String("keys", "", "Protected signing-key directory; root key can be offline during sign")
	out := flag.String("out", "dist/update-repository", "Repository output directory")
	pkg := flag.String("package", "", "Complete exe or deb; omit to renew metadata")
	version := flag.String("version", "", "Client release version")
	osName := flag.String("os", "", "windows or linux")
	arch := flag.String("arch", "", "amd64 or arm64")
	days := flag.Int("days", 7, "Timestamp validity, 1-30 days; renew before expiry")
	flag.Parse()
	if *keys == "" || *days < 1 || *days > 30 {
		fmt.Fprintln(os.Stderr, "provide -keys and a validity between 1 and 30 days")
		os.Exit(2)
	}
	if err := run(*mode, *keys, *out, *pkg, *version, *osName, *arch, *days); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(mode, keys, out, pkg, version, osName, arch string, days int) error {
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	filename := filepath.Join(out, "repository.json")
	repo := model.UpdateRepository{Metadata: map[string]json.RawMessage{}}
	var rotatedKey ed25519.PrivateKey
	if b, err := os.ReadFile(filename); err == nil {
		if err = json.Unmarshal(b, &repo); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if mode == "init" {
		if len(repo.Metadata) > 0 {
			return fmt.Errorf("repository already exists")
		}
		root := metadata.Root(time.Now().UTC().AddDate(1, 0, 0))
		for _, role := range []string{"root", "targets", "snapshot", "timestamp"} {
			_, key, err := ed25519.GenerateKey(nil)
			if err != nil {
				return err
			}
			if err = platform.SavePrivateFile(filepath.Join(keys, role+".bin"), key, false); err != nil {
				return err
			}
			pub, err := metadata.KeyFromPublicKey(key.Public())
			if err != nil {
				return err
			}
			if err = root.Signed.AddKey(pub, role); err != nil {
				return err
			}
		}
		key, err := loadKey(keys, "root")
		if err != nil {
			return err
		}
		signer, err := signature.LoadSigner(key, crypto.Hash(0))
		if err != nil {
			return err
		}
		if _, err = root.Sign(signer); err != nil {
			return err
		}
		b, err := root.ToBytes(false)
		if err != nil {
			return err
		}
		repo.Metadata["root.json"], repo.Metadata["1.root.json"] = b, b
		if err = writeFile(filepath.Join(out, "root.json"), b); err != nil {
			return err
		}
	} else if len(repo.Metadata) == 0 {
		return fmt.Errorf("initialize the repository first")
	}
	if mode == "rotate-root" {
		root, err := metadata.Root().FromBytes(repo.Metadata["root.json"])
		if err != nil {
			return err
		}
		old, err := loadRootKey(keys, root)
		if err != nil {
			return err
		}
		_, next, err := ed25519.GenerateKey(nil)
		if err != nil {
			return err
		}
		pub, err := metadata.KeyFromPublicKey(next.Public())
		if err != nil {
			return err
		}
		root.Signed.Roles["root"].KeyIDs = nil
		if err = root.Signed.AddKey(pub, "root"); err != nil {
			return err
		}
		root.Signed.Version++
		root.Signed.Expires = time.Now().UTC().AddDate(1, 0, 0)
		root.Signatures = nil
		for _, key := range []ed25519.PrivateKey{old, next} {
			signer, e := signature.LoadSigner(key, crypto.Hash(0))
			if e != nil {
				return e
			}
			if _, e = root.Sign(signer); e != nil {
				return e
			}
		}
		b, err := root.ToBytes(false)
		if err != nil {
			return err
		}
		// Stage the next key before committing the repository so an interrupted
		// root.bin replacement can recover from the versioned private key.
		if err = platform.SavePrivateFile(filepath.Join(keys, fmt.Sprintf("root-%d.bin", root.Signed.Version)), next, true); err != nil {
			return err
		}
		rotatedKey = next
		repo.Metadata["root.json"], repo.Metadata[fmt.Sprintf("%d.root.json", root.Signed.Version)] = b, b
	} else if mode != "init" && mode != "sign" {
		return fmt.Errorf("unknown mode")
	}
	targets := metadata.Targets(time.Now().UTC().AddDate(0, 0, days+1))
	if b := repo.Metadata["targets.json"]; len(b) > 0 {
		old, err := metadata.Targets().FromBytes(b)
		if err != nil {
			return err
		}
		targets.Signed.Targets = old.Signed.Targets
		targets.Signed.Version = old.Signed.Version + 1
	}
	if pkg != "" {
		if !model.ValidVersion(version) || !model.ValidUpdatePlatform(osName, arch) {
			return fmt.Errorf("invalid package version/platform")
		}
		target := filepath.Base(pkg)
		t, err := metadata.TargetFile().FromFile(pkg, "sha256")
		if err != nil {
			return err
		}
		custom, _ := json.Marshal(model.UpdateArtifact{Version: version, OS: osName, Arch: arch})
		raw := json.RawMessage(custom)
		t.Custom = &raw
		if old, ok := targets.Signed.Targets[target]; ok {
			if old.Length != t.Length || !bytes.Equal(old.Hashes["sha256"], t.Hashes["sha256"]) || old.Custom == nil || !bytes.Equal(*old.Custom, custom) {
				return fmt.Errorf("immutable target already exists")
			}
		}
		targets.Signed.Targets[target] = t
	}
	snapshot := metadata.Snapshot(time.Now().UTC().AddDate(0, 0, days+1))
	timestamp := metadata.Timestamp(time.Now().UTC().AddDate(0, 0, days))
	if b := repo.Metadata["snapshot.json"]; len(b) > 0 {
		old, e := metadata.Snapshot().FromBytes(b)
		if e != nil {
			return e
		}
		snapshot.Signed.Version = old.Signed.Version + 1
	}
	if b := repo.Metadata["timestamp.json"]; len(b) > 0 {
		old, e := metadata.Timestamp().FromBytes(b)
		if e != nil {
			return e
		}
		timestamp.Signed.Version = old.Signed.Version + 1
	}
	key, err := loadKey(keys, "targets")
	if err != nil {
		return err
	}
	signer, err := signature.LoadSigner(key, crypto.Hash(0))
	if err != nil {
		return err
	}
	if _, err = targets.Sign(signer); err != nil {
		return err
	}
	repo.Metadata["targets.json"], err = targets.ToBytes(false)
	if err != nil {
		return err
	}
	snapshot.Signed.Meta["targets.json"] = metadata.MetaFile(targets.Signed.Version)
	key, err = loadKey(keys, "snapshot")
	if err != nil {
		return err
	}
	signer, err = signature.LoadSigner(key, crypto.Hash(0))
	if err != nil {
		return err
	}
	if _, err = snapshot.Sign(signer); err != nil {
		return err
	}
	repo.Metadata["snapshot.json"], err = snapshot.ToBytes(false)
	if err != nil {
		return err
	}
	timestamp.Signed.Meta["snapshot.json"] = metadata.MetaFile(snapshot.Signed.Version)
	key, err = loadKey(keys, "timestamp")
	if err != nil {
		return err
	}
	signer, err = signature.LoadSigner(key, crypto.Hash(0))
	if err != nil {
		return err
	}
	if _, err = timestamp.Sign(signer); err != nil {
		return err
	}
	repo.Metadata["timestamp.json"], err = timestamp.ToBytes(false)
	if err != nil {
		return err
	}
	u, err := update.VerifyRepository(repo.Metadata["root.json"], repo.Metadata, "")
	if err != nil {
		return err
	}
	for name := range targets.Signed.Targets {
		if _, err = update.Artifact(u, name); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(repo, "", "  ")
	if err != nil {
		return err
	}
	if err = writeFile(filename, b); err != nil {
		return err
	}
	if len(rotatedKey) > 0 {
		if err = platform.SavePrivateFile(filepath.Join(keys, "root.bin"), rotatedKey, true); err != nil {
			return err
		}
	}
	if err = writeFile(filepath.Join(out, "root.json"), repo.Metadata["root.json"]); err != nil {
		return err
	}
	fmt.Println("Signed repository prepared:", filename)
	return nil
}

// A committed repository can survive interruption before root.bin is replaced.
func loadRootKey(dir string, root *metadata.Metadata[metadata.RootType]) (ed25519.PrivateKey, error) {
	for _, name := range []string{fmt.Sprintf("root-%d", root.Signed.Version), "root"} {
		key, e := loadKey(dir, name)
		if e != nil {
			continue
		}
		pub, e := metadata.KeyFromPublicKey(key.Public())
		if e != nil {
			return nil, e
		}
		keyID, e := pub.ID()
		if e != nil {
			return nil, e
		}
		for _, id := range root.Signed.Roles["root"].KeyIDs {
			if id == keyID {
				return key, nil
			}
		}
	}
	return nil, fmt.Errorf("no private key matches the current root role")
}

func loadKey(dir, name string) (ed25519.PrivateKey, error) {
	b, e := platform.LoadPrivateFile(filepath.Join(dir, name+".bin"))
	if e != nil {
		return nil, e
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid signing key")
	}
	return ed25519.PrivateKey(b), nil
}
func writeFile(name string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(name), ".metadata-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, e = f.Write(b); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), name)
}
