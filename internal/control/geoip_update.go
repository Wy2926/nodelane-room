package control

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

const DefaultGeoIPURL = "https://download.db-ip.com/free/dbip-city-lite-{month}.mmdb.gz"
const geoIPMaxBytes = 256 << 20

// AutoGeoIP uses a persistent cache and updates it without delaying HTTP startup.
// The returned stop function drains the updater before releasing its reader.
func AutoGeoIP(ctx context.Context, dir, source string, log *slog.Logger) (*GeoIP, func(), error) {
	u, err := url.Parse(strings.ReplaceAll(source, "{month}", "2026-01"))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, nil, errors.New("GeoIP download URL must use HTTPS without credentials or fragment")
	}
	path := filepath.Join(dir, "geoip.mmdb")
	g, err := OpenGeoIP(path)
	if err != nil {
		g = &GeoIP{}
		if !errors.Is(err, os.ErrNotExist) {
			log.Warn("GeoIP cache unavailable; downloading a replacement")
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		client := &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if r.URL.Scheme != "https" || r.URL.User != nil || len(via) >= 5 {
				return errors.New("invalid GeoIP redirect")
			}
			return nil
		}}
		for {
			delay := 24 * time.Hour
			if err := g.update(ctx, client, path, source, time.Now().UTC()); err != nil {
				if ctx.Err() != nil {
					return
				}
				// Do not log URLs: custom sources can contain signed query values.
				log.Warn("GeoIP update failed; keeping cached data and retrying in one hour")
				delay = time.Hour
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return g, func() { cancel(); <-done; g.Close() }, nil
}

func (g *GeoIP) update(ctx context.Context, client *http.Client, path, source string, now time.Time) error {
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	g.mu.RLock()
	current := g.db != nil && g.db.Metadata.BuildEpoch >= uint(month.Unix())
	g.mu.RUnlock()
	if current && strings.Contains(source, "{month}") {
		return nil
	}
	for i := 0; i < 2; i++ {
		address := strings.ReplaceAll(source, "{month}", month.AddDate(0, -i, 0).Format("2006-01"))
		err := g.download(ctx, client, path, address)
		if !errors.Is(err, os.ErrNotExist) || !strings.Contains(source, "{month}") {
			return err
		}
		if g.available() {
			return err // Keep the cache while a new monthly release is pending.
		}
	}
	return errors.New("GeoIP monthly release unavailable")
}

func (g *GeoIP) download(ctx context.Context, client *http.Client, path, address string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return os.ErrNotExist
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GeoIP HTTP status %d", resp.StatusCode)
	}
	z, err := gzip.NewReader(io.LimitReader(resp.Body, 128<<20))
	if err != nil {
		return err
	}
	defer z.Close()
	b, err := io.ReadAll(io.LimitReader(z, geoIPMaxBytes+1))
	if err != nil {
		return err
	}
	if len(b) > geoIPMaxBytes {
		return errors.New("GeoIP database exceeds size limit")
	}
	db, err := maxminddb.FromBytes(b)
	if err != nil {
		return err
	}
	defer func() {
		if db != nil {
			_ = db.Close()
		}
	}()
	if err = db.Verify(); err != nil {
		return err
	}
	typeName := strings.ToLower(db.Metadata.DatabaseType)
	if !strings.Contains(typeName, "city") && !strings.Contains(typeName, "country") {
		return errors.New("GeoIP source must contain a City or Country database")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".geoip-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	g.mu.Lock()
	old := g.db
	g.db, db = db, nil
	if old != nil {
		_ = old.Close()
	}
	g.mu.Unlock()
	return nil
}
