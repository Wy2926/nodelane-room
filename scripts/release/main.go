// Build the immutable same-origin native installation payload from release binaries.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

type artifact struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func copyFile(src, dst string) error {
	b, e := os.ReadFile(src)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(dst), 0755); e != nil {
		return e
	}
	return os.WriteFile(dst, b, 0644)
}
func run() error {
	release := flag.String("release", "", "node release directory")
	version := flag.String("version", "", "node version")
	installer := flag.String("installer", "deploy/node.sh", "native installer")
	unit := flag.String("unit", "deploy/nlroom-node.service", "systemd unit")
	flag.Parse()
	if !regexp.MustCompile(`^0\.\d+\.\d+$`).MatchString(*version) {
		return fmt.Errorf("invalid version")
	}
	out := filepath.Join(*release, "releases")
	if e := os.MkdirAll(out, 0755); e != nil {
		return e
	}
	manifest := struct {
		Version   string              `json:"version"`
		Artifacts map[string]artifact `json:"artifacts"`
	}{*version, map[string]artifact{}}
	for _, arch := range []string{"amd64", "arm64"} {
		bundle := filepath.Join(*release, "nodelane-room-node-"+*version+"-linux-"+arch)
		files := map[string]string{"nlroom-node": filepath.Join(bundle, "nlroom-node"), "nlroom-node.service": *unit, "THIRD_PARTY_NOTICES.txt": filepath.Join(bundle, "THIRD_PARTY_NOTICES.txt"), "BUILD.txt": filepath.Join(bundle, "BUILD.txt")}
		if e := filepath.WalkDir(filepath.Join(bundle, "licenses"), func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !d.IsDir() {
				r, e := filepath.Rel(bundle, p)
				if e != nil {
					return e
				}
				files[filepath.ToSlash(r)] = p
			}
			return nil
		}); e != nil {
			return e
		}
		name := "nlroom-node-" + *version + "-linux-" + arch + ".tar.gz"
		if e := archive(filepath.Join(out, name), files); e != nil {
			return e
		}
		b, e := os.ReadFile(filepath.Join(out, name))
		if e != nil {
			return e
		}
		sum := sha256.Sum256(b)
		manifest.Artifacts["linux/"+arch] = artifact{name, hex.EncodeToString(sum[:])}
	}
	b, e := json.MarshalIndent(manifest, "", "  ")
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(out, "manifest.json"), append(b, '\n'), 0644); e != nil {
		return e
	}
	if e = copyFile(*installer, filepath.Join(out, "node.sh")); e != nil {
		return e
	}
	sums := ""
	for _, arch := range []string{"amd64", "arm64"} {
		a := manifest.Artifacts["linux/"+arch]
		sums += a.SHA256 + "  " + a.File + "\n"
	}
	for _, name := range []string{"node.sh", "manifest.json"} {
		b, e = os.ReadFile(filepath.Join(out, name))
		if e != nil {
			return e
		}
		sum := sha256.Sum256(b)
		sums += hex.EncodeToString(sum[:]) + "  " + name + "\n"
	}
	if e = os.WriteFile(filepath.Join(out, "SHA256SUMS"), []byte(sums), 0644); e != nil {
		return e
	}
	fmt.Println("Native releases:", out)
	return nil
}
func archive(path string, files map[string]string) error {
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		r, e := os.Open(files[name])
		if e != nil {
			return e
		}
		info, e := r.Stat()
		if e != nil {
			r.Close()
			return e
		}
		if !info.Mode().IsRegular() {
			r.Close()
			return fmt.Errorf("nonregular release input")
		}
		mode := int64(0644)
		if name == "nlroom-node" {
			mode = 0755
		}
		e = tw.WriteHeader(&tar.Header{Name: name, Size: info.Size(), Mode: mode})
		if e == nil {
			_, e = io.Copy(tw, r)
		}
		r.Close()
		if e != nil {
			return e
		}
	}
	if e = tw.Close(); e != nil {
		return e
	}
	if e = gz.Close(); e != nil {
		return e
	}
	return f.Close()
}
