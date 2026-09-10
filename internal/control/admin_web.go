package control

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/nodelane/nodelane-room/internal/platform"
)

//go:embed adminweb/dist
var webFiles embed.FS
var page, _ = webFiles.ReadFile("adminweb/dist/index.html")

var adminPathPattern = regexp.MustCompile(`^/[A-Za-z0-9_-]{16,128}$`)

// ConfigureAdminPath publishes an instance-local entry before HTTP starts.
// Exclusive creation keeps concurrent starts on the same generated path.
func ConfigureAdminPath(dir, configured string) (string, error) {
	if configured != "" && !adminPathPattern.MatchString(configured) {
		return "", errors.New("admin path must be / followed by 16–128 letters, digits, underscores or hyphens")
	}
	path := filepath.Join(dir, "admin-path.bin")
	if configured != "" {
		return configured, platform.SavePrivateFile(path, []byte(configured), true)
	}
	value, err := AdminPath(dir)
	if !errors.Is(err, os.ErrNotExist) {
		return value, err
	}
	value = "/" + randomID()
	if err = platform.SavePrivateFile(path, []byte(value), false); errors.Is(err, os.ErrExist) {
		return AdminPath(dir)
	}
	return value, err
}

func AdminPath(dir string) (string, error) {
	b, err := platform.LoadPrivateFile(filepath.Join(dir, "admin-path.bin"))
	if err != nil {
		return "", err
	}
	if !adminPathPattern.Match(b) {
		return "", errors.New("invalid persisted admin path")
	}
	return string(b), nil
}

func (s *Server) registerAdminWeb(mux *http.ServeMux) {
	if !adminPathPattern.MatchString(s.AdminPath) {
		return // Fail closed when no instance entry has been configured.
	}
	assets, _ := fs.Sub(webFiles, "adminweb/dist/assets")
	prefix := s.AdminPath + "/assets/"
	files := http.StripPrefix(prefix, http.FileServer(http.FS(assets)))
	entries, _ := fs.ReadDir(assets, ".")
	for _, entry := range entries {
		if !entry.IsDir() {
			mux.Handle("GET "+prefix+entry.Name(), files)
		}
	}
	mux.HandleFunc("GET "+s.AdminPath, s.adminPage)
	mux.HandleFunc("GET "+s.AdminPath+"/{$}", s.adminPage)
}

func (s *Server) adminPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(bytes.ReplaceAll(page, []byte(`"./assets/`), []byte(`"`+s.AdminPath+`/assets/`)))
}
