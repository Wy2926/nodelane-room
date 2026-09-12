package update

import (
	"bufio"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type bundle struct {
	version, arch string
	gui           bool
	files         map[string]string
}

var bundleVersion = regexp.MustCompile(`(?m)^Version: (\d+\.\d+\.\d+)\s*$`)
var bundleArch = regexp.MustCompile(`(?m)^Target: windows/(amd64|arm64)\s*$`)

// Manifests describe bytes, not executable commands or destination paths.
func readBundle(dir string) (bundle, error) {
	b := bundle{files: map[string]string{}}
	build, err := os.ReadFile(filepath.Join(dir, "BUILD.txt"))
	if err != nil {
		return b, err
	}
	v, a := bundleVersion.FindSubmatch(build), bundleArch.FindSubmatch(build)
	if len(v) != 2 || len(a) != 2 {
		return b, errors.New("invalid Windows bundle metadata")
	}
	b.version, b.arch = string(v[1]), string(a[1])
	f, err := os.Open(filepath.Join(dir, "PAYLOAD.sha256"))
	if err != nil {
		return b, err
	}
	defer f.Close()
	manifest, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if err != nil || len(manifest) > 2<<20 {
		return b, errors.New("invalid payload manifest")
	}
	seen := map[string]bool{}
	scan := bufio.NewScanner(strings.NewReader(string(manifest)))
	for scan.Scan() {
		parts := strings.SplitN(strings.TrimPrefix(scan.Text(), "\ufeff"), "  ", 2)
		if len(parts) != 2 || !bundleFile(parts[1]) {
			return b, errors.New("invalid payload path")
		}
		digest, err := hex.DecodeString(parts[0])
		key := strings.ToLower(parts[1])
		if err != nil || len(digest) != sha256.Size {
			return b, errors.New("invalid payload digest")
		}
		if seen[key] {
			return b, errors.New("duplicate payload path")
		}
		// Use canonical casing from the manifest after rejecting case aliases.
		seen[key] = true
		b.files[parts[1]] = hex.EncodeToString(digest)
		if key == "build.txt" && sha256.Sum256(build) != [sha256.Size]byte(digest) {
			return b, errors.New("bundle metadata changed")
		}
		if err := checkFileDigest(filepath.Join(dir, filepath.FromSlash(parts[1])), b.files[parts[1]]); err != nil {
			return b, fmt.Errorf("payload %s: %w", parts[1], err)
		}
	}
	if err := scan.Err(); err != nil {
		return b, err
	}
	for _, name := range []string{"nlroom-cli.exe", "nlroom-service.exe", "nlroom-update.exe", "build.txt", "third_party_notices.txt"} {
		if !seen[name] {
			return b, fmt.Errorf("incomplete payload: %s", name)
		}
	}
	if seen["nlroom.exe"] {
		b.gui = true
		data, err := os.ReadFile(filepath.Join(dir, "Uninstall.exe"))
		if err != nil {
			return b, errors.New("missing native uninstaller")
		}
		digest := sha256.Sum256(data)
		b.files["Uninstall.exe"] = hex.EncodeToString(digest[:])
	}
	for _, name := range []string{"nlroom-cli.exe", "nlroom-service.exe", "nlroom-update.exe", "nlroom.exe"} {
		if !seen[name] {
			continue
		}
		binary, err := pe.Open(filepath.Join(dir, name))
		if err != nil {
			return b, fmt.Errorf("invalid executable: %s", name)
		}
		machine := binary.Machine
		binary.Close()
		if (b.arch == "amd64" && machine != pe.IMAGE_FILE_MACHINE_AMD64) || (b.arch == "arm64" && machine != pe.IMAGE_FILE_MACHINE_ARM64) {
			return b, fmt.Errorf("executable architecture mismatch: %s", name)
		}
	}
	digest := sha256.Sum256(manifest)
	b.files["PAYLOAD.sha256"] = hex.EncodeToString(digest[:])
	return b, nil
}

func bundleFile(name string) bool {
	if name == "" || strings.ContainsAny(name, `\:`) || strings.HasPrefix(name, "/") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, ". ") != part {
			return false
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if slices.Contains([]string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}, base) {
			return false
		}
	}
	name = strings.ToLower(name)
	return slices.Contains([]string{"nlroom.exe", "nlroom-cli.exe", "nlroom-service.exe", "nlroom-update.exe", "build.txt", "third_party_notices.txt", "install.cmd", "nodelaneroom.cmd"}, name) || strings.HasPrefix(name, "licenses/") || slices.Contains([]string{"drivers/tap/sha256sums", "drivers/tap/oemvista.inf", "drivers/tap/tap0901.cat", "drivers/tap/tap0901.sys", "drivers/tap/tapctl.exe"}, name)
}

func checkFileDigest(path, digest string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != digest {
		return errors.New("checksum mismatch")
	}
	return nil
}

// overwriteBundle runs only after the service and desktop have exited. Each
// file is verified before replacement. Interrupted installs are repaired by
// running the same complete package again; no old bundle or journal is kept.
func overwriteBundle(source, target string, b bundle, create func(string) (*os.File, error), seal func(string) error) error {
	names := make([]string, 0, len(b.files))
	keep := map[string]bool{}
	for name := range b.files {
		names = append(names, name)
		keep[strings.ToLower(filepath.FromSlash(name))] = true
	}
	slices.Sort(names)
	for _, name := range names {
		dest := filepath.Join(target, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		if err := copyBundleFile(filepath.Join(source, filepath.FromSlash(name)), dest, b.files[name], create, seal); err != nil {
			return fmt.Errorf("replace %s: %w", name, err)
		}
	}
	// Never start a service from a mixed or damaged package.
	if _, err := readBundle(target); err != nil {
		return err
	}
	var stale []string
	if err := filepath.WalkDir(target, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == target {
			return nil
		}
		rel, err := filepath.Rel(target, path)
		if err != nil {
			return err
		}
		if entry.IsDir() || !keep[strings.ToLower(rel)] {
			stale = append(stale, path)
		}
		return seal(path)
	}); err != nil {
		return err
	}
	for i := len(stale) - 1; i >= 0; i-- {
		info, err := os.Stat(stale[i])
		if err != nil {
			return err
		}
		if info.IsDir() {
			entries, err := os.ReadDir(stale[i])
			if err != nil {
				return err
			}
			if len(entries) != 0 {
				continue
			}
		}
		if err := os.Remove(stale[i]); err != nil {
			return err
		}
	}
	return nil
}

func copyBundleFile(from, to, digest string, create func(string) (*os.File, error), seal func(string) error) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := create(filepath.Dir(to))
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), in)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if hex.EncodeToString(h.Sum(nil)) != digest {
		return errors.New("payload changed during installation")
	}
	if err := seal(out.Name()); err != nil {
		return err
	}
	return os.Rename(out.Name(), to)
}
