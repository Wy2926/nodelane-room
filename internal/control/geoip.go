package control

import (
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

type GeoIP struct {
	mu sync.RWMutex
	db *maxminddb.Reader
}

func OpenGeoIP(path string) (*GeoIP, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// An in-memory reader permits atomic cache replacement on Windows as well.
	db, err := maxminddb.FromBytes(b)
	if err != nil {
		return nil, err
	}
	return &GeoIP{db: db}, nil
}
func (g *GeoIP) Close() {
	if g != nil {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.db != nil {
			_ = g.db.Close()
			g.db = nil
		}
	}
}

func (g *GeoIP) provider() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.db == nil {
		return ""
	}
	typeName := strings.ToLower(g.db.Metadata.DatabaseType)
	if strings.Contains(typeName, "dbip") || strings.Contains(typeName, "db-ip") {
		return "dbip"
	}
	if strings.Contains(typeName, "geolite") || strings.Contains(typeName, "geoip2") {
		return "maxmind"
	}
	return ""
}
func (g *GeoIP) available() bool {
	if g == nil {
		return false
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.db != nil
}
func (g *GeoIP) lookup(remote string) (string, string) {
	a, err := netip.ParseAddrPort(remote)
	if err != nil || !a.Addr().IsGlobalUnicast() || a.Addr().IsPrivate() {
		return "", ""
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.db == nil {
		return "", ""
	}
	var record struct {
		Country struct {
			ISO string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
		Subdivisions []struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"subdivisions"`
	}
	if g.db.Lookup(net.IP(a.Addr().AsSlice()), &record) != nil {
		return "", ""
	}
	region := ""
	if len(record.Subdivisions) > 0 {
		region = record.Subdivisions[0].Names["zh-CN"]
		if region == "" {
			region = record.Subdivisions[0].Names["en"]
		}
	}
	country := strings.ToUpper(record.Country.ISO)
	if len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z' {
		country = ""
	}
	return country, region
}
