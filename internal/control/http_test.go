package control

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHTTPRouteBoundaries(t *testing.T) {
	const entry = "/private-entry-for-test"
	h := (&Server{AdminPath: entry, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	for _, tc := range []struct {
		method string
		path   string
		status int
	}{
		{"GET", "/", 404},
		{"HEAD", "/", 404},
		{"GET", "/admin", 404},
		{"GET", "/admin/", 404},
		{"GET", "/admin/assets/app.js", 404},
		{"GET", entry, 200},
		{"HEAD", entry, 200},
		{"GET", entry + "/", 200},
		{"GET", entry + "/assets/app.js", 200},
		{"GET", entry + "/assets/style.css", 200},
		{"GET", entry + "/assets/", 404},
		{"GET", entry + "/assets/index.html", 404},
		{"GET", entry + "/assets/missing.js", 404},
		{"GET", "/install/unknown", 404},
		{"POST", "/v2/rooms/example/unknown", 404},
		{"POST", "/v2/rooms/example/unknown/nested", 404},
		{"POST", "/v2/nodes/enroll", 404},
		{"POST", "/v1/rooms", 404},
		{"POST", entry + "/assets/app.js", 405},
		{"DELETE", "/v2/rooms/example", 405},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
			if tc.path == entry && w.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("admin page missing content policy")
			}
			if tc.status == 404 && (w.Header().Get("Location") != "" || strings.Contains(w.Body.String(), entry)) {
				t.Fatal("public route discloses admin entry")
			}
			if tc.path == entry && tc.method == "GET" && (!strings.Contains(w.Body.String(), entry+"/assets/app.js") || strings.Contains(w.Body.String(), "/admin/")) {
				t.Fatal("page assets do not use the private entry")
			}
		})
	}
}

func TestHTTPContractRequiresAuthentication(t *testing.T) {
	source, err := os.ReadFile("../../docs/openapi.yaml")
	must(t, err)
	var contract struct {
		Security []map[string][]string
		Paths    map[string]map[string]yaml.Node
	}
	must(t, yaml.Unmarshal(source, &contract))
	h := (&Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	checked := 0
	for path, operations := range contract.Paths {
		for method, operation := range operations {
			if !strings.Contains(" get post put patch delete head options ", " "+method+" ") {
				continue
			}
			var spec struct{ Security []map[string][]string }
			must(t, operation.Decode(&spec))
			if spec.Security == nil {
				spec.Security = contract.Security
			}
			if len(spec.Security) == 0 {
				continue
			}
			checked++
			path := strings.NewReplacer("{room}", "example", "{node}", "example").Replace(path)
			methods := []string{strings.ToUpper(method)}
			if method == "get" {
				methods = append(methods, "HEAD")
			}
			for _, method := range methods {
				t.Run(method+" "+path, func(t *testing.T) {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(`{}`)))
					if w.Code != http.StatusUnauthorized {
						t.Fatalf("status = %d, want 401", w.Code)
					}
				})
			}
		}
	}
	if checked == 0 {
		t.Fatal("contract contains no authenticated operations")
	}
}

func TestAdminCookieCannotAuthorizePlayerOrNode(t *testing.T) {
	_, a := newAdmin(t)
	for _, path := range []string{"/v2/rooms/example", "/v2/node/sync"} {
		method := http.MethodGet
		if path == "/v2/node/sync" {
			method = http.MethodPost
		}
		req, err := http.NewRequest(method, a.server.URL+path, nil)
		must(t, err)
		req.AddCookie(a.cookie)
		req.Header.Set("Authorization", "Bearer "+a.cookie.Value)
		resp, err := a.server.Client().Do(req)
		must(t, err)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("admin session authorized %s: %d", path, resp.StatusCode)
		}
	}
}
