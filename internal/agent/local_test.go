package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nodelane/nodelane-room/internal/localapi"
	"github.com/nodelane/nodelane-room/internal/model"
)

func TestPlayerRPCBoundary(t *testing.T) {
	r, err := New(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ body, code string }{
		{`{"action":"status"}`, ""},
		{`{"action":"status"} {}`, "invalid_request"},
		{`{"action":"status","room":"../admin"}`, "invalid_request"},
		{`{"action":"status","extra":true}`, "invalid_request"},
		{`{"action":"service-install"}`, "unknown_action"},
		{`{"action":"games"}`, "unconfigured"},
	} {
		w := httptest.NewRecorder()
		r.localHandler().ServeHTTP(w, httptest.NewRequest("POST", "/rpc", strings.NewReader(tc.body)))
		if tc.code == "" {
			var s model.Status
			if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil || s.ProtocolVersion != localapi.ProtocolVersion {
				t.Fatal("missing local version")
			}
			for _, field := range []string{"private_key", "certificate", "token"} {
				if bytes.Contains(w.Body.Bytes(), []byte(field)) {
					t.Fatal("secret in status")
				}
			}
		} else {
			var e localapi.Error
			if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || e.Code != tc.code {
				t.Fatalf("want %s: %s", tc.code, w.Body.String())
			}
		}
	}
}

func TestArtworkOriginTypeAndLimit(t *testing.T) {
	r, err := New(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aS1sAAAAASUVORK5CYII=")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "" {
			t.Error("image leaked session")
		}
		switch req.URL.Path {
		case "/v2/games/valid/images/cover":
			w.Write(png)
		case "/v2/games/redirect/images/cover":
			http.Redirect(w, req, "/v2/games/valid/images/cover", 302)
		case "/v2/games/large/images/cover":
			w.Write(bytes.Repeat([]byte("x"), (5<<20)+1))
		default:
			w.Write([]byte("<svg onload='bad()'></svg>"))
		}
	}))
	defer server.Close()
	r.status.Server = server.URL
	for _, id := range []string{"../admin", "redirect", "large", "svg"} {
		if _, _, err := r.gameImage(context.Background(), id, "cover"); err == nil {
			t.Fatalf("accepted %s", id)
		}
	}
	b, mime, err := r.gameImage(context.Background(), "valid", "cover")
	if err != nil || mime != "image/png" || !bytes.Equal(b, png) {
		t.Fatal("valid artwork failed", err)
	}
}
