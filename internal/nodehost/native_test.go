package nodehost

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractBinaryRejectsDuplicateAndSymlink(t *testing.T) {
	for _, kind := range []string{"duplicate", "symlink", "missing"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "a.tar.gz")
			f, e := os.Create(archive)
			if e != nil {
				t.Fatal(e)
			}
			gz := gzip.NewWriter(f)
			tw := tar.NewWriter(gz)
			h := &tar.Header{Name: "nlroom-node", Mode: 0755, Size: 1, Typeflag: tar.TypeReg}
			if kind == "symlink" {
				h.Typeflag = tar.TypeSymlink
				h.Size = 0
				h.Linkname = "/etc/passwd"
			}
			if kind == "missing" {
				h.Name = "../../nlroom-node"
			}
			if e = tw.WriteHeader(h); e != nil {
				t.Fatal(e)
			}
			if h.Size > 0 {
				_, _ = tw.Write([]byte("x"))
			}
			if kind == "duplicate" {
				_ = tw.WriteHeader(h)
				_, _ = tw.Write([]byte("y"))
			}
			_ = tw.Close()
			_ = gz.Close()
			_ = f.Close()
			if e = ExtractBinary(archive, filepath.Join(dir, "out")); e == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}
func TestReadSecret(t *testing.T) {
	for _, s := range []string{"short", strings.Repeat("z", 64), strings.Repeat("a", 129)} {
		if _, e := ReadSecret(strings.NewReader(s)); e == nil {
			t.Fatal("bad secret accepted")
		}
	}
	if _, e := ReadSecret(strings.NewReader(strings.Repeat("a", 64) + "\n")); e != nil {
		t.Fatal(e)
	}
}
