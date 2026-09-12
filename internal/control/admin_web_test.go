package control

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nodelane/nodelane-room/internal/platform"
)

func TestAdminPathPersistenceAndRotation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("private control directory is verified in Linux")
	}
	dir := t.TempDir()
	first, err := ConfigureAdminPath(dir, "")
	must(t, err)
	second, err := ConfigureAdminPath(dir, "")
	must(t, err)
	other, err := ConfigureAdminPath(t.TempDir(), "")
	must(t, err)
	if len(first) != 33 || first != second || first == other {
		t.Fatal("random instance path was not persisted independently")
	}
	const configured = "/my-private-admin-entry"
	value, err := ConfigureAdminPath(dir, configured)
	must(t, err)
	value, err = AdminPath(dir)
	must(t, err)
	if value != configured {
		t.Fatal("configured entry was not persisted")
	}
	for _, bad := range []string{"/admin", "/v2", "/", "/private/admin/path", "/private-entry?query", "/private-entry#fragment", "/private-{pattern}", "/private%2fencoded", "/private-entry-测试"} {
		if _, err = ConfigureAdminPath(dir, bad); err == nil {
			t.Fatalf("accepted invalid entry: %s", bad)
		}
	}
	h := (&Server{AdminPath: value, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", first, nil))
	if w.Code != 404 || w.Header().Get("Location") != "" {
		t.Fatal("rotated entry remains accessible")
	}
	must(t, platform.SavePrivateFile(filepath.Join(dir, "admin-path.bin"), []byte("/admin"), true))
	if _, err = ConfigureAdminPath(dir, ""); err == nil {
		t.Fatal("corrupt entry silently accepted")
	}
}

func TestAdminWebWithoutEntryFailsClosed(t *testing.T) {
	h := (&Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	for _, path := range []string{"/admin", "/admin/assets/app.js"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 || w.Header().Get("Location") != "" {
			t.Fatalf("unconfigured web entry exposed at %s", path)
		}
	}
}
