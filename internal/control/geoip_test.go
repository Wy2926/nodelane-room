package control

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestGeoIPFixture(t *testing.T) {
	path := os.Getenv("NODELANE_TEST_GEOIP_DB")
	if path == "" {
		t.Skip("set NODELANE_TEST_GEOIP_DB to MaxMind GeoIP2-City-Test.mmdb")
	}
	g, err := OpenGeoIP(path)
	must(t, err)
	defer g.Close()
	for _, remote := range []string{"81.2.69.160:4242", "[::ffff:81.2.69.160]:4242"} {
		country, region := g.lookup(remote)
		if country != "GB" || region == "" {
			t.Fatalf("fixture lookup %s = %s/%s", remote, country, region)
		}
	}
	for _, remote := range []string{"10.0.0.1:1234", "127.0.0.1:1234", "invalid"} {
		country, region := g.lookup(remote)
		if country != "" || region != "" {
			t.Fatal("non-public address geolocated")
		}
	}
}

func TestGeoIPDownloadFailureKeepsCache(t *testing.T) {
	for name, body := range map[string][]byte{"invalid-gzip": []byte("bad"), "invalid-mmdb": gzipData(t, []byte("bad database")), "truncated-gzip": gzipData(t, []byte("bad"))[:12]} {
		t.Run(name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
			defer s.Close()
			path := filepath.Join(t.TempDir(), "geoip.mmdb")
			must(t, os.WriteFile(path, []byte("existing cache"), 0600))
			g := &GeoIP{}
			if err := g.download(context.Background(), s.Client(), path, s.URL); err == nil {
				t.Fatal("invalid download accepted")
			}
			data, err := os.ReadFile(path)
			must(t, err)
			if string(data) != "existing cache" || g.available() {
				t.Fatal("failed download replaced cache")
			}
		})
	}
}

func gzipData(t *testing.T, b []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	z := gzip.NewWriter(&out)
	_, err := z.Write(b)
	must(t, err)
	must(t, z.Close())
	return out.Bytes()
}

func TestGeoIPAutomaticUpdateAndConcurrentLookup(t *testing.T) {
	fixture := os.Getenv("NODELANE_TEST_GEOIP_DB")
	if fixture == "" {
		t.Skip("set NODELANE_TEST_GEOIP_DB to MaxMind GeoIP2-City-Test.mmdb")
	}
	b, err := os.ReadFile(fixture)
	must(t, err)
	body := gzipData(t, b)
	var requests []string
	var requestsMu sync.Mutex
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		requests = append(requests, r.URL.Path)
		requestsMu.Unlock()
		if r.URL.Path == "/2026-09" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write(body)
	}))
	defer s.Close()
	path := filepath.Join(t.TempDir(), "geoip.mmdb")
	g := &GeoIP{}
	defer g.Close()
	must(t, g.update(context.Background(), s.Client(), path, s.URL+"/{month}", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)))
	requestsMu.Lock()
	fallback := len(requests) == 2 && requests[1] == "/2026-08"
	requestsMu.Unlock()
	if !fallback || !g.available() {
		t.Fatal("first download did not fall back to the previous monthly release")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				country, region := g.lookup("81.2.69.160:4242")
				if country != "GB" || region == "" || g.provider() != "maxmind" || !g.available() {
					t.Error("concurrent lookup lost the active database")
				}
			}
		})
	}
	for range 3 {
		must(t, g.download(context.Background(), s.Client(), path, s.URL+"/update"))
	}
	wg.Wait()
	cached, err := OpenGeoIP(path)
	must(t, err)
	defer cached.Close()
	if country, _ := cached.lookup("81.2.69.160:4242"); country != "GB" {
		t.Fatal("updated cache cannot be restored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = g.download(ctx, s.Client(), path, s.URL); err == nil || !g.available() {
		t.Fatal("cancelled update damaged active database")
	}
}

func TestGeoIPAutomaticSourceValidation(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, source := range []string{"", "http://example.com/db.gz", "https://user:password@example.com/db.gz", "https://example.com/db.gz#fragment"} {
		if _, _, err := AutoGeoIP(context.Background(), t.TempDir(), source, log); err == nil {
			t.Fatal("invalid automatic source accepted")
		}
	}
}
