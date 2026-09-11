package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

func ValidURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == "" && len(value) <= 8192
}

// Signed packages may be mirrored but authorization headers never follow them.
var downloadTransport = &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 15 * time.Second}).DialContext, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 90 * time.Second, MaxIdleConns: 32, MaxIdleConnsPerHost: 4}

func HTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Minute, Transport: downloadTransport, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 4 || !ValidURL(r.URL.String()) {
			return errors.New("update_redirect_rejected")
		}
		return nil
	}}
}

func VerifyFile(name string, artifact model.UpdateArtifact) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	s, err := f.Stat()
	if err != nil {
		return err
	}
	if !s.Mode().IsRegular() || s.Size() != artifact.Size {
		return errors.New("update_size_mismatch")
	}
	h := sha256.New()
	if _, err = io.Copy(h, io.LimitReader(f, artifact.Size+1)); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != artifact.SHA256 {
		return errors.New("update_hash_mismatch")
	}
	return nil
}

// Download resumes only the same immutable digest; switching mirrors is safe
// because the complete result must match the signed size and SHA256.
func Download(ctx context.Context, client *http.Client, urls []string, file string, a model.UpdateArtifact, progress func(int64)) error {
	if a.Size <= 0 || a.Size > MaxPackageSize || len(urls) == 0 || len(urls) > 16 {
		return errors.New("update_download_unavailable")
	}
	if VerifyFile(file, a) == nil {
		progress(a.Size)
		return nil
	}
	partial := file + ".partial"
	for attempt := 0; attempt < 3; attempt++ {
		for _, address := range urls {
			if !ValidURL(address) {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := downloadOnce(ctx, client, address, partial, a.Size, progress); err != nil {
				continue
			}
			if VerifyFile(partial, a) != nil {
				_ = os.Remove(partial)
				continue
			}
			if err := os.Rename(partial, file); err != nil {
				return err
			}
			return nil
		}
		t := time.NewTimer(time.Duration(attempt+1) * time.Second)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	return errors.New("update_download_failed")
}

func downloadOnce(ctx context.Context, client *http.Client, address, file string, size int64, progress func(int64)) error {
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	s, err := f.Stat()
	if err != nil {
		return err
	}
	offset := s.Size()
	if offset > size {
		if err = f.Truncate(0); err != nil {
			return err
		}
		offset = 0
	}
	if offset == size {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	res, err := client.Do(req)
	if err != nil {
		return errors.New("update_source_unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusOK {
		if err = f.Truncate(0); err != nil {
			return err
		}
		offset = 0
	} else if res.StatusCode == http.StatusPartialContent && offset > 0 {
		want := fmt.Sprintf("bytes %d-%d/%d", offset, size-1, size)
		if res.Header.Get("Content-Range") != want {
			return errors.New("update_range_invalid")
		}
	} else {
		return errors.New("update_source_unavailable")
	}
	if enc := res.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
		return errors.New("update_encoding_invalid")
	}
	if v := res.Header.Get("Content-Length"); v != "" {
		n, e := strconv.ParseInt(v, 10, 64)
		if e != nil || n != size-offset {
			return errors.New("update_size_mismatch")
		}
	}
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	buf := make([]byte, 128<<10)
	r := io.LimitReader(res.Body, size-offset+1)
	for {
		n, e := r.Read(buf)
		if offset+int64(n) > size {
			return errors.New("update_size_mismatch")
		}
		if n > 0 {
			written, werr := f.Write(buf[:n])
			if werr != nil {
				return werr
			}
			if written != n {
				return io.ErrShortWrite
			}
			offset += int64(n)
			progress(offset)
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
	}
	if offset != size {
		return io.ErrUnexpectedEOF
	}
	return f.Sync()
}

func ObjectKey(prefix, target string) string {
	return strings.Trim(strings.Trim(prefix, "/")+"/"+target, "/")
}

var cachedPackageName = regexp.MustCompile(`^[0-9a-f]{64}\.(exe|deb)(\.partial)?$`)

// Keep only the current job and next candidate. Never recurse into cache or
// touch identity files, and never remove a package used by an active worker.
func PrunePackages(dir string, keep []model.UpdateArtifact) error {
	names := map[string]bool{}
	for _, a := range keep {
		name := filepath.Base(PackagePath(dir, a))
		names[name] = true
		names[name+".partial"] = true
	}
	base := filepath.Join(dir, "updates")
	entries, e := os.ReadDir(base)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if !entry.IsDir() && cachedPackageName.MatchString(entry.Name()) && !names[entry.Name()] {
			if e = os.Remove(filepath.Join(base, entry.Name())); e != nil {
				return e
			}
		}
	}
	return nil
}
