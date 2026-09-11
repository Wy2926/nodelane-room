package update

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/theupdateframework/go-tuf/v2/metadata"
)

func signedFixture(t *testing.T, key ed25519.PrivateKey, v int64, expires time.Time) ([]byte, map[string]json.RawMessage, model.UpdateArtifact) {
	t.Helper()
	signer, err := signature.LoadSigner(key, crypto.Hash(0))
	if err != nil {
		t.Fatal(err)
	}
	pub, err := metadata.KeyFromPublicKey(key.Public())
	if err != nil {
		t.Fatal(err)
	}
	root := metadata.Root(time.Now().Add(time.Hour))
	for _, role := range []string{"root", "timestamp", "snapshot", "targets"} {
		if err = root.Signed.AddKey(pub, role); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = root.Sign(signer); err != nil {
		t.Fatal(err)
	}
	rb, err := root.ToBytes(false)
	if err != nil {
		t.Fatal(err)
	}
	targets := metadata.Targets(expires)
	targets.Signed.Version = v
	payload := []byte("signed package")
	h := sha256.Sum256(payload)
	a := model.UpdateArtifact{Version: "0.3.0", OS: "windows", Arch: "amd64", Target: "nlroom-0.3.0.exe", Size: int64(len(payload)), SHA256: hex.EncodeToString(h[:])}
	tf, err := metadata.TargetFile().FromBytes(a.Target, payload, "sha256")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(model.UpdateArtifact{Version: a.Version, OS: a.OS, Arch: a.Arch})
	custom := json.RawMessage(b)
	tf.Custom = &custom
	targets.Signed.Targets[a.Target] = tf
	if _, err = targets.Sign(signer); err != nil {
		t.Fatal(err)
	}
	tb, _ := targets.ToBytes(false)
	snapshot := metadata.Snapshot(expires)
	snapshot.Signed.Version = v
	snapshot.Signed.Meta["targets.json"] = metadata.MetaFile(v)
	if _, err = snapshot.Sign(signer); err != nil {
		t.Fatal(err)
	}
	sb, _ := snapshot.ToBytes(false)
	timestamp := metadata.Timestamp(expires)
	timestamp.Signed.Version = v
	timestamp.Signed.Meta["snapshot.json"] = metadata.MetaFile(v)
	if _, err = timestamp.Sign(signer); err != nil {
		t.Fatal(err)
	}
	ts, _ := timestamp.ToBytes(false)
	return rb, map[string]json.RawMessage{"root.json": rb, "1.root.json": rb, "targets.json": tb, "snapshot.json": sb, "timestamp.json": ts}, a
}

func TestTrustedMetadataRejectsTamperExpiryAndRollback(t *testing.T) {
	_, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	root, bundle, a := signedFixture(t, key, 2, time.Now().Add(time.Hour))
	cache := t.TempDir()
	u, err := VerifyRepository(root, bundle, cache)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Artifact(u, a.Target)
	if err != nil || got != a {
		t.Fatalf("target mismatch: %v %v", got, err)
	}
	_, older, _ := signedFixture(t, key, 1, time.Now().Add(time.Hour))
	if _, err = VerifyRepository(root, older, cache); err == nil {
		t.Fatal("accepted rollback")
	}
	_, expired, _ := signedFixture(t, key, 3, time.Now().Add(-time.Hour))
	if _, err = VerifyRepository(root, expired, cache); err == nil {
		t.Fatal("accepted expired metadata")
	}
	var target map[string]any
	if err = json.Unmarshal(bundle["targets.json"], &target); err != nil {
		t.Fatal(err)
	}
	target["signed"].(map[string]any)["version"] = 99
	bundle["targets.json"], _ = json.Marshal(target)
	if _, err = VerifyRepository(root, bundle, t.TempDir()); err == nil {
		t.Fatal("accepted modified metadata")
	}
}

func TestRepositoryAdvanceIsolatedAndRootChecked(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(nil)
	root, old, _ := signedFixture(t, key, 1, time.Now().Add(time.Hour))
	_, next, _ := signedFixture(t, key, 2, time.Now().Add(time.Hour))
	// Reuse exactly the root; it is immutable within its version.
	next["root.json"], next["1.root.json"] = root, root
	if _, err := VerifyAdvance(root, old, next); err != nil {
		t.Fatal(err)
	}
	_, other, _ := ed25519.GenerateKey(nil)
	badRoot, _, _ := signedFixture(t, other, 2, time.Now().Add(time.Hour))
	next["root.json"] = badRoot
	if _, err := VerifyAdvance(root, old, next); err == nil {
		t.Fatal("accepted substituted root")
	}
}
