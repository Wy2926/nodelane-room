package model

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Release versions are numeric major.minor.patch, independent of IPC/LAN versions.
func ValidVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 || len(v) > 32 {
		return false
	}
	for _, p := range parts {
		if p == "" || (len(p) > 1 && p[0] == '0') {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
		if _, err := strconv.ParseUint(p, 10, 32); err != nil {
			return false
		}
	}
	return true
}

func CompareVersion(a, b string) int {
	for i, p := range strings.Split(a, ".") {
		if i > 2 || !ValidVersion(a) || !ValidVersion(b) {
			return 0
		}
		x, _ := strconv.ParseUint(p, 10, 32)
		y, _ := strconv.ParseUint(strings.Split(b, ".")[i], 10, 32)
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

func ValidUpdatePlatform(os, arch string) bool {
	return (os == "windows" || os == "linux") && (arch == "amd64" || arch == "arm64")
}

type ClientReport struct {
	Version    string `json:"version"`
	GUIVersion string `json:"gui_version,omitempty"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	State      string `json:"state"`
	ReleaseID  string `json:"release_id,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
}

type DeviceSoftware struct {
	ClientReport
	ReportedAt time.Time `json:"reported_at"`
}

// Source secrets are write-only and never returned in lists or audit events.
type UpdateSource struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Endpoint       string `json:"endpoint,omitempty"`
	Bucket         string `json:"bucket,omitempty"`
	Region         string `json:"region,omitempty"`
	Prefix         string `json:"prefix,omitempty"`
	PublicURL      string `json:"public_url,omitempty"`
	PathStyle      bool   `json:"path_style"`
	Enabled        bool   `json:"enabled"`
	Priority       int    `json:"priority"`
	Revision       int64  `json:"revision"`
	HasCredentials bool   `json:"has_credentials"`
	AccessKey      string `json:"access_key,omitempty"`
	SecretKey      string `json:"secret_key,omitempty"`
}

type UpdateArtifact struct {
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Target  string `json:"target"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

type UpdateRelease struct {
	UpdateArtifact
	ID        string    `json:"id"`
	Notes     string    `json:"notes"`
	State     string    `json:"state"`
	Revision  int64     `json:"revision"`
	Sources   []string  `json:"sources"`
	CreatedAt time.Time `json:"created_at"`
}

type UpdatePolicy struct {
	OS             string     `json:"os"`
	Arch           string     `json:"arch"`
	ReleaseID      string     `json:"release_id"`
	MinimumVersion string     `json:"minimum_version,omitempty"`
	EffectiveAt    *time.Time `json:"effective_at,omitempty"`
	Revision       int64      `json:"revision"`
}

type UpdateRepository struct {
	Revision int64                      `json:"revision"`
	Metadata map[string]json.RawMessage `json:"metadata"`
}

type UpdateCheck struct {
	ServerTime time.Time         `json:"server_time"`
	Policy     *UpdatePolicy     `json:"policy,omitempty"`
	Release    *UpdateRelease    `json:"release,omitempty"`
	Required   bool              `json:"required"`
	URLs       []string          `json:"urls"`
	Repository *UpdateRepository `json:"repository,omitempty"`
}

// DownloadRelease contains only the public installer details.
type DownloadRelease struct {
	UpdateArtifact
	ID          string    `json:"id"`
	Notes       string    `json:"notes"`
	CreatedAt   time.Time `json:"created_at"`
	Recommended bool      `json:"recommended"`
}

type DownloadCatalog struct {
	ServerTime time.Time         `json:"server_time"`
	Releases   []DownloadRelease `json:"releases"`
}

type DownloadLinks struct {
	Release DownloadRelease `json:"release"`
	URLs    []string        `json:"urls"`
}

type UpdateStatus struct {
	State      string         `json:"state"`
	ErrorCode  string         `json:"error_code,omitempty"`
	Downloaded int64          `json:"downloaded"`
	CheckedAt  time.Time      `json:"checked_at"`
	Required   bool           `json:"required"`
	Policy     *UpdatePolicy  `json:"policy,omitempty"`
	Release    *UpdateRelease `json:"release,omitempty"`
}

type UpdateDevice struct {
	DeviceID string         `json:"device_id"`
	UserID   string         `json:"user_id"`
	Name     string         `json:"name"`
	Software DeviceSoftware `json:"software"`
}

type UpdateOverview struct {
	Sources            []UpdateSource  `json:"sources"`
	Releases           []UpdateRelease `json:"releases"`
	Policies           []UpdatePolicy  `json:"policies"`
	RepositoryRevision int64           `json:"repository_revision"`
	Devices            []UpdateDevice  `json:"devices"`
	Versions           []VersionCount  `json:"versions"`
	Attempts           []UpdateAttempt `json:"attempts"`
	Next               string          `json:"next,omitempty"`
}

type VersionCount struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Version string `json:"version"`
	Devices int64  `json:"devices"`
	Fresh   int64  `json:"fresh"`
}

type UpdateAttempt struct {
	DeviceID  string    `json:"device_id"`
	ReleaseID string    `json:"release_id"`
	State     string    `json:"state"`
	ErrorCode string    `json:"error_code,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}
