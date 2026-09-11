package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/update"
)

var ErrUpdateRequired = errors.New("client update required")

func readRepository(ctx context.Context, tx pgx.Tx) (model.UpdateRepository, error) {
	var r model.UpdateRepository
	var b []byte
	err := tx.QueryRow(ctx, "SELECT metadata,revision FROM update_repository WHERE id=1").Scan(&b, &r.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r.Metadata)
	return r, err
}

func controlUpdateRoot() ([]byte, error) {
	if path := os.Getenv("NODELANE_UPDATE_ROOT"); path != "" {
		return os.ReadFile(path)
	}
	return update.TrustedRoot()
}

func saveRepository(ctx context.Context, tx pgx.Tx, actor string, in model.UpdateRepository) (any, error) {
	old, err := readRepository(ctx, tx)
	if err != nil {
		return nil, err
	}
	if old.Revision != in.Revision {
		return nil, ErrConflict
	}
	root, err := controlUpdateRoot()
	if err != nil {
		return nil, err
	}
	u, err := update.VerifyAdvance(root, old.Metadata, in.Metadata)
	if err != nil {
		return nil, fmt.Errorf("%w: signed repository rejected", ErrInvalid)
	}
	// Metadata renewal must retain every published or policy-referenced target.
	rows, err := tx.Query(ctx, "SELECT artifact FROM update_releases WHERE state IN ('published','paused') OR id IN (SELECT release_id FROM update_policies)")
	if err != nil {
		return nil, err
	}
	artifacts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.UpdateArtifact, error) {
		var a model.UpdateArtifact
		var b []byte
		e := row.Scan(&b)
		if e == nil {
			e = json.Unmarshal(b, &a)
		}
		return a, e
	})
	if err != nil {
		return nil, err
	}
	for _, a := range artifacts {
		got, e := update.Artifact(u, a.Target)
		if e != nil || got != a {
			return nil, fmt.Errorf("%w: retain active release targets", ErrConflict)
		}
	}
	in.Revision++
	b, _ := json.Marshal(in.Metadata)
	_, err = tx.Exec(ctx, "INSERT INTO update_repository(id,metadata,revision) VALUES(1,$1,$2) ON CONFLICT(id) DO UPDATE SET metadata=$1,revision=$2", b, in.Revision)
	if err == nil {
		err = adminEvent(ctx, tx, actor, "update.repository_imported", "", map[string]int64{"revision": in.Revision})
	}
	return map[string]int64{"revision": in.Revision}, err
}

func readRelease(ctx context.Context, tx pgx.Tx, id string) (model.UpdateRelease, error) {
	var r model.UpdateRelease
	var b []byte
	err := tx.QueryRow(ctx, "SELECT id,artifact,notes,state,revision,created_at FROM update_releases WHERE id=$1", id).Scan(&r.ID, &b, &r.Notes, &r.State, &r.Revision, &r.CreatedAt)
	if err != nil {
		return r, noRows(err)
	}
	if err = json.Unmarshal(b, &r.UpdateArtifact); err != nil {
		return r, err
	}
	rows, err := tx.Query(ctx, `SELECT p.source_id FROM update_replicas p JOIN update_sources s ON s.id=p.source_id WHERE release_id=$1 AND p.source_revision=s.revision ORDER BY s.config->>'priority',p.source_id`, id)
	if err != nil {
		return r, err
	}
	r.Sources, err = pgx.CollectRows(rows, pgx.RowTo[string])
	return r, err
}

func saveRelease(ctx context.Context, tx pgx.Tx, actor string, in model.UpdateRelease) (any, error) {
	if len(in.Notes) > 16000 || !slices.Contains([]string{"draft", "published", "paused", "withdrawn"}, in.State) {
		return nil, ErrInvalid
	}
	if in.ID == "" {
		if in.State != "draft" || in.Revision != 0 {
			return nil, ErrInvalid
		}
		repo, err := readRepository(ctx, tx)
		if err != nil {
			return nil, err
		}
		root, err := controlUpdateRoot()
		if err != nil {
			return nil, err
		}
		u, err := update.VerifyRepository(root, repo.Metadata, "")
		if err != nil {
			return nil, ErrInvalid
		}
		a, err := update.Artifact(u, in.Target)
		if err != nil {
			return nil, ErrInvalid
		}
		in.ID = randomID()
		in.UpdateArtifact = a
		in.Revision = 1
		b, _ := json.Marshal(a)
		if _, err = tx.Exec(ctx, `INSERT INTO update_releases(id,version,os,arch,artifact,notes,state) VALUES($1,$2,$3,$4,$5,$6,'draft')`, in.ID, a.Version, a.OS, a.Arch, b, in.Notes); err != nil {
			return nil, err
		}
	} else {
		old, err := readRelease(ctx, tx, in.ID)
		if err != nil {
			return nil, err
		}
		if old.Revision != in.Revision || old.UpdateArtifact != in.UpdateArtifact {
			return nil, ErrConflict
		}
		var referenced bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM update_policies WHERE release_id=$1)", in.ID).Scan(&referenced); err != nil {
			return nil, err
		}
		if referenced && in.State != "published" {
			return nil, fmt.Errorf("%w: switch or remove the policy before pausing this release", ErrConflict)
		}
		if old.State == "withdrawn" && in.State != "withdrawn" {
			return nil, ErrConflict
		}
		if in.State == "published" {
			if err = verifyReleaseMetadata(ctx, tx, old); err != nil {
				return nil, err
			}
			if len(old.Sources) == 0 {
				return nil, fmt.Errorf("%w: verify at least one source first", ErrConflict)
			}
			if _, err = releaseURLs(ctx, tx, old); err != nil {
				return nil, err
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE update_releases SET notes=$2,state=$3,revision=revision+1 WHERE id=$1", in.ID, in.Notes, in.State); err != nil {
			return nil, err
		}
	}
	if err := adminEvent(ctx, tx, actor, "update.release_saved", in.ID, map[string]string{"state": in.State}); err != nil {
		return nil, err
	}
	return readRelease(ctx, tx, in.ID)
}

func savePolicy(ctx context.Context, tx pgx.Tx, actor string, p model.UpdatePolicy) (any, error) {
	if !model.ValidUpdatePlatform(p.OS, p.Arch) || (p.MinimumVersion != "" && (!model.ValidVersion(p.MinimumVersion) || p.EffectiveAt == nil)) {
		return nil, ErrInvalid
	}
	var rev int64
	err := tx.QueryRow(ctx, "SELECT revision FROM update_policies WHERE os=$1 AND arch=$2", p.OS, p.Arch).Scan(&rev)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if rev != p.Revision {
		return nil, ErrConflict
	}
	if p.ReleaseID == "" {
		p.Revision++
		p.MinimumVersion = ""
		p.EffectiveAt = nil
		_, err = tx.Exec(ctx, `INSERT INTO update_policies(os,arch,release_id,minimum_version,effective_at,revision) VALUES($1,$2,NULL,'',NULL,$3) ON CONFLICT(os,arch) DO UPDATE SET release_id=NULL,minimum_version='',effective_at=NULL,revision=$3`, p.OS, p.Arch, p.Revision)
	} else {
		r, e := readRelease(ctx, tx, p.ReleaseID)
		if e != nil {
			return nil, e
		}
		if e = verifyReleaseMetadata(ctx, tx, r); e != nil {
			return nil, e
		}
		if r.State != "published" || r.OS != p.OS || r.Arch != p.Arch || (p.MinimumVersion != "" && model.CompareVersion(p.MinimumVersion, r.Version) > 0) {
			return nil, ErrInvalid
		}
		if _, err = releaseURLs(ctx, tx, r); err != nil {
			return nil, err
		}
		p.Revision++
		_, err = tx.Exec(ctx, `INSERT INTO update_policies(os,arch,release_id,minimum_version,effective_at,revision) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(os,arch) DO UPDATE SET release_id=$3,minimum_version=$4,effective_at=$5,revision=$6`, p.OS, p.Arch, p.ReleaseID, p.MinimumVersion, p.EffectiveAt, p.Revision)
	}
	if err == nil {
		err = enforceUpdates(ctx, tx)
	}
	if err == nil {
		err = adminEvent(ctx, tx, actor, "update.policy_saved", p.OS+"/"+p.Arch, p)
	}
	return p, err
}

func releaseURLs(ctx context.Context, tx pgx.Tx, r model.UpdateRelease) ([]string, error) {
	sources := []model.UpdateSource{}
	for _, id := range r.Sources {
		src, err := readSource(ctx, tx, id, true)
		if err != nil {
			return nil, err
		}
		if src.Enabled {
			sources = append(sources, src)
		}
	}
	slices.SortFunc(sources, func(a, b model.UpdateSource) int { return a.Priority - b.Priority })
	urls := []string{}
	for _, src := range sources {
		address, err := sourceURL(ctx, src, r.Target)
		if err != nil {
			return nil, err
		}
		urls = append(urls, address)
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("%w: no verified enabled update source", ErrConflict)
	}
	return urls, nil
}

func verifyReleaseMetadata(ctx context.Context, tx pgx.Tx, r model.UpdateRelease) error {
	repo, err := readRepository(ctx, tx)
	if err != nil {
		return err
	}
	root, err := controlUpdateRoot()
	if err != nil {
		return err
	}
	u, err := update.VerifyRepository(root, repo.Metadata, "")
	if err != nil {
		return fmt.Errorf("%w: renew valid signed metadata before publication", ErrConflict)
	}
	a, err := update.Artifact(u, r.Target)
	if err != nil || a != r.UpdateArtifact {
		return ErrConflict
	}
	return nil
}

func readPolicy(ctx context.Context, tx pgx.Tx, osName, arch string) (*model.UpdatePolicy, error) {
	var p model.UpdatePolicy
	err := tx.QueryRow(ctx, "SELECT os,arch,COALESCE(release_id,''),minimum_version,effective_at,revision FROM update_policies WHERE os=$1 AND arch=$2", osName, arch).Scan(&p.OS, &p.Arch, &p.ReleaseID, &p.MinimumVersion, &p.EffectiveAt, &p.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

func updateRequired(ctx context.Context, tx pgx.Tx, device string) (bool, error) {
	var b []byte
	err := tx.QueryRow(ctx, "SELECT report FROM device_software WHERE device_id=$1", device).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		var required bool
		err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM update_policies WHERE minimum_version<>'' AND effective_at<=now())").Scan(&required)
		return required, err
	}
	if err != nil {
		return false, err
	}
	var r model.ClientReport
	if err = json.Unmarshal(b, &r); err != nil {
		return false, err
	}
	p, err := readPolicy(ctx, tx, r.OS, r.Arch)
	if err != nil || p == nil {
		return false, err
	}
	return p.MinimumVersion != "" && p.EffectiveAt != nil && !time.Now().Before(*p.EffectiveAt) && (!model.ValidVersion(r.Version) || model.CompareVersion(r.Version, p.MinimumVersion) < 0 || (r.GUIVersion != "" && model.CompareVersion(r.GUIVersion, p.MinimumVersion) < 0)), nil
}

func requireUpdated(ctx context.Context, tx pgx.Tx, id string) error {
	required, err := updateRequired(ctx, tx, id)
	if err != nil {
		return err
	}
	if required {
		return ErrUpdateRequired
	}
	return nil
}

func enforceUpdates(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, "SELECT DISTINCT device_id FROM members WHERE active")
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, id := range ids {
		required, e := updateRequired(ctx, tx, id)
		if e != nil {
			return e
		}
		if !required {
			continue
		}
		rows, e := tx.Query(ctx, "SELECT room_id FROM members WHERE active AND device_id=$1", id)
		if e != nil {
			return e
		}
		rooms, e := pgx.CollectRows(rows, pgx.RowTo[string])
		if e != nil {
			return e
		}
		for _, room := range rooms {
			if e = revoke(ctx, tx, room, id); e != nil {
				return e
			}
			if e = bump(ctx, tx, room, "update_required"); e != nil {
				return e
			}
		}
	}
	return nil
}

func reportClient(ctx context.Context, tx pgx.Tx, id string, in model.ClientReport) error {
	if !model.ValidVersion(in.Version) || !model.ValidUpdatePlatform(in.OS, in.Arch) || (in.GUIVersion != "" && !model.ValidVersion(in.GUIVersion)) || !slices.Contains([]string{"idle", "checking", "available", "downloading", "ready", "installing", "succeeded", "failed", "rolled_back", "unconfigured"}, in.State) || len(in.ErrorCode) > 80 {
		return ErrInvalid
	}
	for _, c := range in.ErrorCode {
		if !(c >= 'a' && c <= 'z' || c == '_') {
			return ErrInvalid
		}
	}
	if _, err := playerUser(ctx, tx, id); err != nil {
		return err
	}
	b, _ := json.Marshal(in)
	_, err := tx.Exec(ctx, `INSERT INTO device_software(device_id,report) VALUES($1,$2) ON CONFLICT(device_id) DO UPDATE SET report=$2,reported_at=now()`, id, b)
	if err != nil {
		return err
	}
	if in.ReleaseID != "" {
		r, e := readRelease(ctx, tx, in.ReleaseID)
		if e != nil {
			return e
		}
		if in.State == "succeeded" && (r.Version != in.Version || r.OS != in.OS || r.Arch != in.Arch) {
			return ErrInvalid
		}
		_, err = tx.Exec(ctx, `INSERT INTO update_attempts(device_id,release_id,state,error_code) VALUES($1,$2,$3,$4) ON CONFLICT(device_id,release_id) DO UPDATE SET state=$3,error_code=$4,updated_at=now() WHERE update_attempts.state<>$3 OR update_attempts.error_code<>$4`, id, in.ReleaseID, in.State, in.ErrorCode)
	}
	return err
}
