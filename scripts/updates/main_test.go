package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/update"
)

func TestSignedRepositoryLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("private signing keys require administrator-controlled ProgramData; run the isolated Linux test")
	}
	base := t.TempDir()
	keys, out := filepath.Join(base, "keys"), filepath.Join(base, "repo")
	if e := run("init", keys, out, "", "", "", "", 7); e != nil {
		t.Fatal(e)
	}
	root, e := os.ReadFile(filepath.Join(out, "root.json"))
	if e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(base, "nlroom_0.3.1_amd64.deb")
	if e = os.WriteFile(file, []byte("signed package fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = run("sign", keys, out, file, "0.3.1", "linux", "amd64", 7); e != nil {
		t.Fatal(e)
	}
	read := func() model.UpdateRepository {
		t.Helper()
		b, e := os.ReadFile(filepath.Join(out, "repository.json"))
		if e != nil {
			t.Fatal(e)
		}
		var r model.UpdateRepository
		if e = json.Unmarshal(b, &r); e != nil {
			t.Fatal(e)
		}
		return r
	}
	old := read()
	cache := filepath.Join(base, "cache")
	u, e := update.VerifyRepository(root, old.Metadata, cache)
	if e != nil {
		t.Fatal(e)
	}
	a, e := update.Artifact(u, filepath.Base(file))
	if e != nil || a.Version != "0.3.1" {
		t.Fatalf("artifact: %+v %v", a, e)
	}
	if e = run("rotate-root", keys, out, "", "", "", "", 7); e != nil {
		t.Fatal(e)
	}
	if _, e = update.VerifyRepository(root, read().Metadata, cache); e != nil {
		t.Fatal(e)
	}
	if _, e = update.VerifyRepository(root, old.Metadata, cache); e == nil {
		t.Fatal("accepted rollback after root rotation")
	}
	if e = run("sign", keys, out, "", "", "", "", 7); e != nil {
		t.Fatal(e)
	}
	if _, e = update.VerifyRepository(root, read().Metadata, cache); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(file, []byte("tampered"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = run("sign", keys, out, file, "0.3.1", "linux", "amd64", 7); e == nil {
		t.Fatal("accepted target overwrite")
	}
}
