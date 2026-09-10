package control

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/nodelane/nodelane-room/internal/model"
)

//go:embed adminweb/*
var webFiles embed.FS
var page = template.Must(template.ParseFS(webFiles, "adminweb/index.html"))

func (s *Server) registerAdminWeb(mux *http.ServeMux) {
	assets, _ := fs.Sub(webFiles, "adminweb")
	files := http.StripPrefix("/admin/assets/", http.FileServer(http.FS(assets)))
	for _, name := range []string{"app.js", "style.css"} {
		mux.Handle("GET /admin/assets/"+name, files)
	}
	mux.HandleFunc("GET /admin", s.adminPage)
	mux.HandleFunc("GET /{$}", s.adminRedirect)
}

func (s *Server) adminPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = page.Execute(w, map[string]string{"Version": model.Version})
}

func (s *Server) adminRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
