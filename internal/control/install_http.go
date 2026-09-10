package control

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

var releaseName = regexp.MustCompile(`^(manifest\.json|node\.sh|nlroom-node-0\.[0-9]+\.[0-9]+-linux-(amd64|arm64)\.tar\.gz|SHA256SUMS)$`)

func (s *Server) registerInstall(mux *http.ServeMux) {
	mux.HandleFunc("GET /install/{file}", s.installFile)
}

func (s *Server) installFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if s.ReleaseDir == "" || !releaseName.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	p := filepath.Join(s.ReleaseDir, name)
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, p)
}
