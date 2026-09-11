package update

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nodelane/nodelane-room/internal/localapi"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
)

type Prepared struct {
	Release    model.UpdateRelease    `json:"release"`
	Repository model.UpdateRepository `json:"repository"`
}
type InstallJob struct {
	Prepared
	From      string    `json:"from"`
	Previous  *Prepared `json:"previous,omitempty"`
	State     string    `json:"state"`
	ErrorCode string    `json:"error_code,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

func PackagePath(dir string, a model.UpdateArtifact) string {
	return filepath.Join(dir, "updates", a.SHA256+filepath.Ext(a.Target))
}
func SaveJob(dir string, j InstallJob) error {
	b, e := json.Marshal(j)
	if e != nil {
		return e
	}
	return platform.SavePrivateFile(filepath.Join(dir, "update-job.bin"), b, true)
}
func LoadJob(dir string) (InstallJob, error) {
	var j InstallJob
	b, e := platform.LoadPrivateFile(filepath.Join(dir, "update-job.bin"))
	if e == nil {
		e = json.Unmarshal(b, &j)
	}
	return j, e
}

func InstallActive(dir string) bool {
	unlock, err := installLock(dir)
	if err != nil {
		return true
	}
	unlock()
	return false
}

func VerifyPrepared(dir string, p Prepared) error {
	root, err := TrustedRoot()
	if err != nil {
		return err
	}
	u, err := VerifyRepository(root, p.Repository.Metadata, filepath.Join(dir, "updates", "metadata"))
	if err != nil {
		return err
	}
	a, err := Artifact(u, p.Release.Target)
	if err != nil || a != p.Release.UpdateArtifact || a.OS != runtime.GOOS || a.Arch != runtime.GOARCH {
		return model.Failure("local_update_package_invalid")
	}
	return VerifyFile(PackagePath(dir, a), a)
}

func WaitReady(ctx context.Context, dir, version string) error {
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	for {
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		var s model.Status
		err := localapi.Call(probe, dir, localapi.Request{Action: "status"}, &s)
		cancel()
		if err == nil && s.Version == version && s.ProtocolVersion == localapi.ProtocolVersion {
			return nil
		}
		t := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-deadline.C:
			t.Stop()
			return model.Failure("local_update_health_failed")
		case <-t.C:
		}
	}
}

func Apply(ctx context.Context, dir string) error {
	if err := platform.SecureDir(dir); err != nil {
		return err
	}
	unlock, err := installLock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	j, err := LoadJob(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if j.State != "ready" && j.State != "installing" {
		return nil
	}
	defer finishInstall()
	fail := func(code string) error {
		j.State, j.ErrorCode = "failed", code
		if e := SaveJob(dir, j); e != nil {
			return e
		}
		return model.Failure(code)
	}
	if !model.ValidVersion(j.From) || model.CompareVersion(j.Release.Version, j.From) <= 0 {
		return fail("local_update_downgrade_rejected")
	}
	if err = VerifyPrepared(dir, j.Prepared); err != nil {
		return fail("local_update_package_invalid")
	}
	if j.Previous != nil {
		if j.Previous.Release.Version != j.From {
			return fail("local_update_rollback_invalid")
		}
		if err = VerifyPrepared(dir, *j.Previous); err != nil {
			return fail("local_update_rollback_invalid")
		}
	}
	j.State = "installing"
	j.StartedAt = time.Now().UTC()
	if err = SaveJob(dir, j); err != nil {
		return err
	}
	err = installPackage(ctx, dir, j)
	if err == nil {
		err = WaitReady(ctx, dir, j.Release.Version)
	}
	if err != nil {
		j.State, j.ErrorCode = "failed", "local_update_recovery_failed"
		if rollbackPackage(ctx, dir, j) == nil && WaitReady(ctx, dir, j.From) == nil {
			j.State = "rolled_back"
			j.ErrorCode = "local_update_rolled_back"
		}
	} else {
		j.State, j.ErrorCode = "succeeded", ""
	}
	if saveErr := SaveJob(dir, j); saveErr != nil {
		return saveErr
	}
	return err
}
