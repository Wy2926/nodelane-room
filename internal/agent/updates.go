package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/nodelane/nodelane-room/internal/localapi"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
	"github.com/nodelane/nodelane-room/internal/update"
)

func (r *Runtime) updateStatus() model.UpdateStatus {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()
	s := r.updateState
	s.Required = requiredLocally(s.Policy)
	return s
}
func requiredLocally(p *model.UpdatePolicy) bool {
	return p != nil && p.MinimumVersion != "" && p.EffectiveAt != nil && !time.Now().Before(*p.EffectiveAt) && model.CompareVersion(model.ClientVersion, p.MinimumVersion) < 0
}
func (r *Runtime) setUpdateState(state, code string) {
	r.updateMu.Lock()
	r.updateState.State, r.updateState.ErrorCode = state, code
	r.updateMu.Unlock()
}

func (r *Runtime) runUpdates(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		r.updateStep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-r.updateWake:
		}
	}
}

func (r *Runtime) updateStep(ctx context.Context) {
	if r.nodeMode {
		return
	}
	r.op.Lock()
	server := r.identity.Server
	r.op.Unlock()
	if server == "" {
		return
	}
	if job, err := update.LoadJob(r.dir); err == nil {
		if job.State == "installing" {
			if time.Since(job.StartedAt) > 2*time.Minute && !update.InstallActive(r.dir) {
				job.State = "ready"
				if update.SaveJob(r.dir, job) == nil {
					r.updateMu.Lock()
					r.updateState.Release = &job.Release
					r.updateState.State = "ready"
					r.updateMu.Unlock()
				}
				return
			}
			r.setUpdateState("installing", "")
			return
		}
		if job.State == "succeeded" || job.State == "rolled_back" || job.State == "failed" {
			r.updateMu.Lock()
			r.updateState.Release = &job.Release
			r.updateMu.Unlock()
			r.setUpdateState(job.State, job.ErrorCode)
			// Do not repeatedly retry a failed release unattended.
		}
	}
	check, err := r.fetchUpdate(ctx, server, false)
	if err != nil {
		r.setUpdateState("failed", "update_check_failed")
		return
	}
	if delta := time.Since(check.ServerTime); delta > 30*time.Second || delta < -30*time.Second {
		r.setUpdateState("failed", "update_clock_invalid")
		return
	}
	r.updateMu.Lock()
	r.updateState.Policy = check.Policy
	r.updateState.CheckedAt = time.Now().UTC()
	r.updateMu.Unlock()
	// Persist even a future enforcement deadline, independently of downloads.
	b, _ := json.Marshal(check.Policy)
	if platform.SavePrivateFile(filepath.Join(r.dir, "update-policy.bin"), b, true) != nil {
		r.setUpdateState("failed", "update_storage_failed")
		return
	}
	if check.Release == nil {
		s := r.updateStatus()
		if s.State != "succeeded" && s.State != "rolled_back" {
			r.setUpdateState("idle", "")
		}
		return
	}
	previousStatus := r.updateStatus()
	r.updateMu.Lock()
	r.updateState.Release = check.Release
	r.updateMu.Unlock()
	if job, e := update.LoadJob(r.dir); e == nil && job.Release.ID == check.Release.ID && (job.State == "failed" || job.State == "rolled_back") && previousStatus.State != "checking" {
		return
	}
	root, err := update.TrustedRoot()
	if err != nil {
		r.setUpdateState("unconfigured", "update_trust_unconfigured")
		return
	}
	if err = platform.SecureDir(r.dir); err != nil {
		r.setUpdateState("failed", "update_storage_failed")
		return
	}
	if err = os.MkdirAll(filepath.Join(r.dir, "updates"), 0700); err != nil {
		r.setUpdateState("failed", "update_storage_failed")
		return
	}
	if check.Repository == nil {
		r.setUpdateState("failed", "update_metadata_invalid")
		return
	}
	cache := filepath.Join(r.dir, "updates", "metadata")
	u, err := update.VerifyRepository(root, check.Repository.Metadata, cache)
	if err != nil {
		r.setUpdateState("failed", "update_metadata_invalid")
		return
	}
	a, err := update.Artifact(u, check.Release.Target)
	if err != nil || a != check.Release.UpdateArtifact || a.OS != runtime.GOOS || a.Arch != runtime.GOARCH || model.CompareVersion(a.Version, model.ClientVersion) <= 0 {
		r.setUpdateState("failed", "update_package_invalid")
		return
	}
	keep := []model.UpdateArtifact{a}
	if old, e := update.LoadJob(r.dir); e == nil {
		keep = append(keep, old.Release.UpdateArtifact)
		if old.Previous != nil {
			keep = append(keep, old.Previous.Release.UpdateArtifact)
		}
	}
	if e := update.PrunePackages(r.dir, keep); e != nil {
		r.setUpdateState("failed", "update_storage_failed")
		return
	}
	space, err := update.FreeSpace(r.dir)
	if err != nil || space < uint64(a.Size)*4+(256<<20) {
		r.setUpdateState("failed", "update_disk_full")
		return
	}
	r.setUpdateState("downloading", "")
	err = update.Download(ctx, update.HTTPClient(), check.URLs, update.PackagePath(r.dir, a), a, func(n int64) { r.updateMu.Lock(); r.updateState.Downloaded = n; r.updateMu.Unlock() })
	if err != nil {
		r.setUpdateState("failed", "update_download_failed")
		return
	}
	job := update.InstallJob{Prepared: update.Prepared{Release: *check.Release, Repository: *check.Repository}, From: model.ClientVersion, State: "ready"}
	if runtime.GOOS == "linux" {
		old, e := r.fetchUpdate(ctx, server, true)
		if e != nil || old.Release == nil || old.Repository == nil || old.Release.Version != model.ClientVersion {
			r.setUpdateState("failed", "update_rollback_unavailable")
			return
		}
		u, e := update.VerifyRepository(root, old.Repository.Metadata, cache)
		if e != nil {
			r.setUpdateState("failed", "update_metadata_invalid")
			return
		}
		previous, e := update.Artifact(u, old.Release.Target)
		if e != nil || previous != old.Release.UpdateArtifact {
			r.setUpdateState("failed", "update_package_invalid")
			return
		}
		if e = update.Download(ctx, update.HTTPClient(), old.URLs, update.PackagePath(r.dir, previous), previous, func(int64) {}); e != nil {
			r.setUpdateState("failed", "update_rollback_unavailable")
			return
		}
		job.Previous = &update.Prepared{Release: *old.Release, Repository: *old.Repository}
		// Use the latest metadata for both targets; never rewind a trusted cache.
		job.Repository = *old.Repository
	}
	// Publication may have changed during a long download. Recheck before making
	// the package installable; the signed bytes alone do not authorize rollout.
	latest, e := r.fetchUpdate(ctx, server, false)
	if e != nil {
		r.setUpdateState("failed", "update_check_failed")
		return
	}
	if latest.Release == nil || latest.Release.ID != job.Release.ID {
		job.State, job.ErrorCode = "failed", "update_release_changed"
		_ = update.SaveJob(r.dir, job)
		r.setUpdateState("failed", "update_release_changed")
		return
	}
	if err = update.SaveJob(r.dir, job); err != nil {
		r.setUpdateState("failed", "update_storage_failed")
		return
	}
	r.setUpdateState("ready", "")
	if runtime.GOOS == "linux" && update.SupportedInstall(r.dir) {
		r.op.Lock()
		idle := r.identity.RoomID == ""
		r.op.Unlock()
		if idle || requiredLocally(check.Policy) {
			_, _ = r.installUpdate(ctx)
		}
	}
}

func (r *Runtime) fetchUpdate(ctx context.Context, server string, installed bool) (model.UpdateCheck, error) {
	var out model.UpdateCheck
	q := url.Values{"version": {model.ClientVersion}, "os": {runtime.GOOS}, "arch": {runtime.GOARCH}}
	if installed {
		q.Set("installed", "1")
	}
	request, err := http.NewRequestWithContext(ctx, "GET", server+"/v2/updates/check?"+q.Encode(), nil)
	if err != nil {
		return out, err
	}
	h := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := h.Do(request)
	if err != nil {
		return out, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return out, errors.New("update_check_failed")
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (3<<20)+1))
	if err != nil || len(b) > 3<<20 {
		return out, errors.New("update_metadata_too_large")
	}
	err = json.Unmarshal(b, &out)
	return out, err
}

func (r *Runtime) installUpdate(ctx context.Context) (any, error) {
	r.op.Lock()
	defer r.op.Unlock()
	r.updateMu.Lock()
	defer r.updateMu.Unlock()
	if r.updateState.State != "ready" {
		return nil, localapi.Failure("update_not_ready", "update is not ready")
	}
	if !update.SupportedInstall(r.dir) {
		return nil, localapi.Failure("update_install_unsupported", "install the complete desktop package")
	}
	job, err := update.LoadJob(r.dir)
	if err != nil || job.State != "ready" {
		return nil, localapi.Failure("update_not_ready", "update is not ready")
	}
	job.State, job.StartedAt = "installing", time.Now().UTC()
	if err = update.SaveJob(r.dir, job); err != nil {
		return nil, localapi.Failure("update_storage_failed", "could not save install job")
	}
	r.updateState.State = "installing"
	if runtime.GOOS == "windows" {
		return map[string]bool{"elevate": true}, nil
	}
	if err = update.StartInstall(ctx, r.dir); err != nil {
		job.State = "ready"
		_ = update.SaveJob(r.dir, job)
		r.updateState.State = "ready"
		return nil, localapi.Failure("update_install_failed", "could not start the update task")
	}
	return map[string]bool{"elevate": false}, nil
}

func (r *Runtime) updateAction(ctx context.Context, in localapi.Request) (any, error) {
	if in.Action == "update-status" {
		return r.updateStatus(), nil
	}
	if in.Action == "update-install" {
		return r.installUpdate(ctx)
	}
	if in.Action == "update-cancel-install" {
		r.updateMu.Lock()
		if r.updateState.State == "installing" && !update.InstallActive(r.dir) {
			if j, e := update.LoadJob(r.dir); e == nil {
				j.State = "ready"
				_ = update.SaveJob(r.dir, j)
			}
			r.updateState.State = "ready"
		}
		r.updateMu.Unlock()
		return map[string]bool{"ok": true}, nil
	}
	if state := r.updateStatus().State; state == "downloading" || state == "installing" {
		return r.updateStatus(), nil
	}
	r.setUpdateState("checking", "")
	select {
	case r.updateWake <- struct{}{}:
	default:
	}
	return r.updateStatus(), nil
}

// Called with r.op held, even outside rooms. Progress samples stay local.
func (r *Runtime) reportVersion(ctx context.Context) {
	if r.api == nil || r.nodeMode {
		return
	}
	s := r.updateStatus()
	r.updateMu.Lock()
	gui := r.guiVersion
	r.updateMu.Unlock()
	state := s.State
	if state == "" {
		state = "idle"
	}
	report := model.ClientReport{Version: model.ClientVersion, GUIVersion: gui, OS: runtime.GOOS, Arch: runtime.GOARCH, State: state, ErrorCode: s.ErrorCode}
	if s.Release != nil {
		report.ReleaseID = s.Release.ID
	}
	if time.Since(r.versionReportedAt) < time.Minute && report == r.lastVersionReport {
		return
	}
	if err := r.api.Call(ctx, "POST", "/v2/client/report", report, nil); err == nil {
		r.versionReportedAt = time.Now()
		r.lastVersionReport = report
	}
}
