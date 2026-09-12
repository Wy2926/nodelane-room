package control

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/nodelane/nodelane-room/internal/model"
	"gopkg.in/yaml.v3"
)

func TestJSONRequestDecoding(t *testing.T) {
	for _, tc := range []struct {
		name, media, body, code string
	}{
		{"omitted type", "", `{"name":"ok"}`, ""},
		{"parameters", "application/json; charset=utf-8", `{"name":"ok"}`, ""},
		{"case insensitive type", "Application/JSON", `{"name":"ok"}`, ""},
		{"type prefix", "application/json-invalid", `{}`, "request_media_unsupported"},
		{"invalid parameter", "application/json; charset", `{}`, "request_media_unsupported"},
		{"other type", "text/plain", `{}`, "request_media_unsupported"},
		{"unknown field", "application/json", `{"unknown":true}`, "request_malformed"},
		{"trailing document", "application/json", `{} {}`, "request_malformed"},
		{"empty body", "application/json", "", "request_malformed"},
		{"body limit", "application/json", `{"name":"` + strings.Repeat("x", 65536) + `"}`, "request_too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.media)
			var in struct {
				Name string `json:"name"`
			}
			err := decodeRequest(httptest.NewRecorder(), r, &in)
			if (tc.code == "" && err != nil) || (tc.code != "" && !model.IsCode(err, tc.code)) {
				t.Fatalf("wanted %q, got %v", tc.code, err)
			}
		})
	}
}

func TestSSEFramesThroughResponseWrappers(t *testing.T) {
	for _, id := range []string{"", "42"} {
		w := httptest.NewRecorder()
		wrapped := &adminResponse{ResponseWriter: &contractWriter{ResponseWriter: w}}
		must(t, writeSSE(wrapped, "snapshot", id, map[string]string{"text": "one\ntwo"}))
		want := "event: snapshot\ndata: {\"text\":\"one\\ntwo\"}\n\n"
		if id != "" {
			want = "id: " + id + "\n" + want
		}
		if w.Body.String() != want || !w.Flushed {
			t.Fatalf("frame %q, flushed %v", w.Body.String(), w.Flushed)
		}
	}
	w := httptest.NewRecorder()
	if err := writeSSE(w, "snapshot", "1", make(chan int)); err == nil || w.Body.Len() != 0 {
		t.Fatal("encoding failure wrote a partial event")
	}
	for _, phase := range []string{"write", "flush"} {
		broken := &sseFailureWriter{ResponseWriter: httptest.NewRecorder()}
		if phase == "write" {
			broken.writeErr = io.ErrClosedPipe
		} else {
			broken.flushErr = io.ErrClosedPipe
		}
		wrapped := &adminResponse{ResponseWriter: &contractWriter{ResponseWriter: broken}}
		if err := writeSSE(wrapped, "snapshot", "1", nil); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("%s failure was lost: %v", phase, err)
		}
	}
}

type sseFailureWriter struct {
	http.ResponseWriter
	writeErr, flushErr error
}

func (w *sseFailureWriter) Write(b []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.ResponseWriter.Write(b)
}

func (w *sseFailureWriter) FlushError() error { return w.flushErr }

func TestHTTPRouteBoundaries(t *testing.T) {
	const entry = "/private-entry-for-test"
	h := (&Server{AdminPath: entry, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
	for _, tc := range []struct {
		method string
		path   string
		status int
	}{
		{"GET", "/", 200},
		{"HEAD", "/", 200},
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
			h.ServeHTTP(w, contractRequest(tc.method, tc.path, nil))
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

func TestHTTPContractRejectionUsesCommonResponseHandling(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v2/downloads", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("contract rejection omitted common response headers: %v", w.Header())
	}
	if s.requests.Load() != 1 || s.failures.Load() != 1 {
		t.Errorf("contract rejection counted %d requests and %d failures", s.requests.Load(), s.failures.Load())
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
					h.ServeHTTP(w, contractRequest(method, path, strings.NewReader(`{}`)))
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
		req.Header.Set(model.ContractHeader, model.Contract)
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
