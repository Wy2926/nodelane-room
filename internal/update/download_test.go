package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nodelane/nodelane-room/internal/model"
)

func TestDownloadResumesAndVerifiesMirror(t *testing.T) {
	payload := "signed package"
	sum := sha256.Sum256([]byte(payload))
	a := model.UpdateArtifact{Size: int64(len(payload)), SHA256: hex.EncodeToString(sum[:])}
	seen := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(503)
			return
		}
		if r.Header.Get("Range") != "bytes=6-" {
			t.Errorf("missing resume range: %s", r.Header.Get("Range"))
		}
		seen = true
		w.Header().Set("Content-Range", "bytes 6-13/14")
		w.WriteHeader(206)
		_, _ = w.Write([]byte(payload[6:]))
	}))
	defer srv.Close()
	file := filepath.Join(t.TempDir(), "package")
	if err := os.WriteFile(file+".partial", []byte(payload[:6]), 0600); err != nil {
		t.Fatal(err)
	}
	err := Download(context.Background(), srv.Client(), []string{srv.URL + "/bad", srv.URL + "/ok"}, file, a, func(int64) {})
	if err != nil || !seen {
		t.Fatal(err)
	}
	if err = VerifyFile(file, a); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadRejectsOverflowAndWrongRange(t *testing.T) {
	for _, bad := range []string{"overflow", "range"} {
		t.Run(bad, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if bad == "range" {
					w.Header().Set("Content-Range", "bytes 0-1/2")
					w.WriteHeader(206)
				}
				_, _ = w.Write([]byte(strings.Repeat("x", 32)))
			}))
			defer srv.Close()
			file := filepath.Join(t.TempDir(), "partial")
			_ = os.WriteFile(file, []byte("x"), 0600)
			if err := downloadOnce(context.Background(), srv.Client(), srv.URL, file, 2, func(int64) {}); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
}
