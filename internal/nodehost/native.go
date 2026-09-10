// Package nodehost contains native installation lifecycle operations. Container
// deployments deliberately use their external orchestrator instead.
package nodehost

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/nodelane/nodelane-room/internal/agent"
	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/model"
)

type Config struct {
	Server  string `json:"server"`
	Version string `json:"version"`
}
type Artifact struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Version   string              `json:"version"`
	Artifacts map[string]Artifact `json:"artifacts"`
}

var versionPattern = regexp.MustCompile(`^0\.[0-9]+\.[0-9]+$`)

func ReadConfig(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	if err == nil {
		err = client.ValidateURL(c.Server)
	}
	return c, err
}
func ReadSecret(r io.Reader) (string, error) {
	b, err := io.ReadAll(io.LimitReader(r, 128))
	if err != nil {
		return "", err
	}
	defer clear(b)
	s := strings.TrimSpace(string(b))
	if len(s) != 64 {
		return "", errors.New("enrollment key must contain 64 hexadecimal characters")
	}
	if _, err = hex.DecodeString(s); err != nil {
		return "", errors.New("invalid enrollment key")
	}
	return s, nil
}
func NativeCheck() error {
	if runtime.GOOS != "linux" {
		return errors.New("native service management requires Debian/Ubuntu Linux")
	}
	if os.Getenv("NLROOM_DEPLOYMENT") == "container" {
		return errors.New("容器部署请使用 Compose 或 1Panel 管理服务、升级及卸载")
	}
	for _, p := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(p); err == nil {
			return errors.New("container detected; use Compose or 1Panel")
		}
	}
	if os.Geteuid() != 0 {
		return errors.New("此操作需要在宿主机 root 终端运行")
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return errors.New("systemd is required")
	}
	return nil
}
func Service(ctx context.Context, action string) error {
	if err := NativeCheck(); err != nil {
		return err
	}
	if action != "start" && action != "stop" && action != "restart" {
		return errors.New("invalid service action")
	}
	return systemctl(ctx, action, "nlroom-node.service")
}
func systemctl(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func download(ctx context.Context, url string, w io.Writer, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	c := &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("release download returned HTTP %d", res.StatusCode)
	}
	n, err := io.Copy(w, io.LimitReader(res.Body, limit+1))
	if err == nil && n > limit {
		return errors.New("release response exceeds size limit")
	}
	return err
}
func ExtractBinary(archive, target string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	found := false
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if h.Name != "nlroom-node" {
			continue
		}
		if found || h.Typeflag != tar.TypeReg || h.Size < 1 || h.Size > 128<<20 {
			return errors.New("invalid node binary archive")
		}
		out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
		if e != nil {
			return e
		}
		_, e = io.Copy(out, tr)
		if e == nil {
			e = out.Sync()
		}
		ce := out.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		found = true
	}
	if !found {
		return errors.New("archive does not contain nlroom-node")
	}
	return nil
}
func Update(ctx context.Context, configPath, stateDir, version string) error {
	if err := NativeCheck(); err != nil {
		return err
	}
	if !versionPattern.MatchString(version) {
		return errors.New("specify an exact release version, for example 0.2.0")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe != "/usr/local/bin/nlroom-node" {
		return errors.New("update must run from /usr/local/bin/nlroom-node")
	}
	cfg, err := ReadConfig(configPath)
	if err != nil {
		return err
	}
	var raw strings.Builder
	if err = download(ctx, cfg.Server+"/install/manifest.json", &raw, 65536); err != nil {
		return err
	}
	var manifest Manifest
	if err = json.Unmarshal([]byte(raw.String()), &manifest); err != nil {
		return err
	}
	if manifest.Version != version {
		return errors.New("requested version is not advertised by this control server")
	}
	artifact, ok := manifest.Artifacts["linux/"+runtime.GOARCH]
	if !ok || artifact.File != "nlroom-node-"+version+"-linux-"+runtime.GOARCH+".tar.gz" || len(artifact.SHA256) != 64 {
		return errors.New("release manifest is invalid")
	}
	lock, err := os.OpenFile("/etc/nlroom-node/update.lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("another update may be running; inspect /etc/nlroom-node/update.lock")
	}
	lock.Close()
	defer os.Remove("/etc/nlroom-node/update.lock")
	dir, err := os.MkdirTemp("/usr/local/bin", ".nlroom-update-")
	if err != nil {
		return err
	}
	keepRecovery := false
	defer func() {
		if !keepRecovery {
			_ = os.RemoveAll(dir)
		}
	}()
	archive := filepath.Join(dir, "release.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		return err
	}
	sum := sha256.New()
	err = download(ctx, cfg.Server+"/install/"+artifact.File, io.MultiWriter(f, sum), 256<<20)
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	if hex.EncodeToString(sum.Sum(nil)) != strings.ToLower(artifact.SHA256) {
		return errors.New("release checksum mismatch")
	}
	next := filepath.Join(dir, "nlroom-node")
	if err = ExtractBinary(archive, next); err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, next, "version").Output()
	if err != nil || !strings.HasPrefix(string(out), version+" ") {
		return errors.New("release binary version check failed")
	}
	var previous model.NodeLocalStatus
	_ = agent.LocalCall(ctx, stateDir, agent.Request{Action: "status"}, &previous)
	if err = systemctl(ctx, "stop", "nlroom-node.service"); err != nil {
		return err
	}
	backup := filepath.Join(dir, "previous")
	if err = os.Rename(exe, backup); err != nil {
		_ = systemctl(ctx, "start", "nlroom-node.service")
		return err
	}
	rollback := func(cause error) error {
		recovery, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_ = systemctl(recovery, "stop", "nlroom-node.service")
		if e := os.Rename(backup, exe); e != nil {
			keepRecovery = true
			return fmt.Errorf("update failed (%v); rollback binary failed (backup retained at %s): %w", cause, backup, e)
		}
		if e := systemctl(recovery, "start", "nlroom-node.service"); e != nil {
			return fmt.Errorf("update failed (%v); restored binary but service start failed: %w", cause, e)
		}
		return fmt.Errorf("update failed; previous binary restored: %w", cause)
	}
	if err = os.Rename(next, exe); err != nil {
		return rollback(err)
	}
	if err = systemctl(ctx, "start", "nlroom-node.service"); err != nil {
		return rollback(err)
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		var s model.NodeLocalStatus
		e := agent.LocalCall(ctx, stateDir, agent.Request{Action: "status"}, &s)
		if e == nil && s.Version == version && (previous.Engine != "running" || s.Engine == "running") {
			fmt.Fprintln(os.Stdout, "Node updated to", version)
			return nil
		}
		select {
		case <-ctx.Done():
			return rollback(ctx.Err())
		case <-time.After(time.Second):
		}
	}
	return rollback(errors.New("new service did not become healthy"))
}
func Uninstall(ctx context.Context) error {
	if err := NativeCheck(); err != nil {
		return err
	}
	if err := systemctl(ctx, "disable", "--now", "nlroom-node.service"); err != nil {
		return err
	}
	for _, p := range []string{"/etc/systemd/system/nlroom-node.service", "/usr/local/bin/nlroom-node"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "已卸载程序和服务；保留 /etc/nlroom-node、/var/lib/nlroom-node 及服务账号。请在控制台撤销不再使用的节点。")
	return nil
}
