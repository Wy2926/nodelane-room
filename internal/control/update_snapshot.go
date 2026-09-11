package control

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Store) checkUpdate(ctx context.Context, version, osName, arch string, installed bool) (model.UpdateCheck, error) {
	out := model.UpdateCheck{ServerTime: time.Now().UTC(), URLs: []string{}}
	err := s.Write(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT now()").Scan(&out.ServerTime); err != nil {
			return err
		}
		if installed {
			var id string
			if err := tx.QueryRow(ctx, "SELECT id FROM update_releases WHERE version=$1 AND os=$2 AND arch=$3 AND state IN ('published','paused')", version, osName, arch).Scan(&id); err != nil {
				return noRows(err)
			}
			release, err := readRelease(ctx, tx, id)
			if err != nil {
				return err
			}
			out.Release = &release
			repo, err := readRepository(ctx, tx)
			if err != nil {
				return err
			}
			out.Repository = &repo
			out.URLs, err = releaseURLs(ctx, tx, release)
			return err
		}
		p, err := readPolicy(ctx, tx, osName, arch)
		if err != nil || p == nil {
			return err
		}
		out.Policy = p
		if p.ReleaseID == "" {
			return nil
		}
		out.Required = p.MinimumVersion != "" && p.EffectiveAt != nil && !out.ServerTime.Before(*p.EffectiveAt) && model.CompareVersion(version, p.MinimumVersion) < 0
		release, err := readRelease(ctx, tx, p.ReleaseID)
		if err != nil {
			return err
		}
		if release.State != "published" || model.CompareVersion(version, release.Version) >= 0 {
			return nil
		}
		out.Release = &release
		out.Repository = &model.UpdateRepository{}
		*out.Repository, err = readRepository(ctx, tx)
		if err != nil {
			return err
		}
		out.URLs, err = releaseURLs(ctx, tx, release)
		return err
	})
	return out, err
}

func (s *Store) updateOverview(ctx context.Context, after string) (model.UpdateOverview, error) {
	out := model.UpdateOverview{Sources: []model.UpdateSource{}, Releases: []model.UpdateRelease{}, Policies: []model.UpdatePolicy{}, Devices: []model.UpdateDevice{}}
	if after != "" && !validResourceID(after) && len(after) != 64 {
		return out, ErrInvalid
	}
	err := s.Write(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT id FROM update_sources ORDER BY id")
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, id := range ids {
			src, e := readSource(ctx, tx, id, false)
			if e != nil {
				return e
			}
			out.Sources = append(out.Sources, src)
		}
		rows, err = tx.Query(ctx, "SELECT id FROM update_releases ORDER BY created_at DESC LIMIT 200")
		if err != nil {
			return err
		}
		ids, err = pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, id := range ids {
			v, e := readRelease(ctx, tx, id)
			if e != nil {
				return e
			}
			out.Releases = append(out.Releases, v)
		}
		for _, osName := range []string{"windows", "linux"} {
			for _, arch := range []string{"amd64", "arm64"} {
				p, e := readPolicy(ctx, tx, osName, arch)
				if e != nil {
					return e
				}
				if p != nil {
					out.Policies = append(out.Policies, *p)
				}
			}
		}
		repo, err := readRepository(ctx, tx)
		if err != nil {
			return err
		}
		out.RepositoryRevision = repo.Revision
		rows, err = tx.Query(ctx, `SELECT s.device_id,u.id,u.name,s.report,s.reported_at FROM device_software s JOIN user_devices d ON d.device_id=s.device_id JOIN users u ON u.id=d.user_id WHERE s.device_id>$1 ORDER BY s.device_id LIMIT 101`, after)
		if err != nil {
			return err
		}
		out.Devices, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.UpdateDevice, error) {
			var d model.UpdateDevice
			var b []byte
			e := row.Scan(&d.DeviceID, &d.UserID, &d.Name, &b, &d.Software.ReportedAt)
			if e == nil {
				e = json.Unmarshal(b, &d.Software.ClientReport)
			}
			return d, e
		})
		if err != nil {
			return err
		}
		if len(out.Devices) > 100 {
			out.Devices = out.Devices[:100]
			out.Next = out.Devices[99].DeviceID
		}
		rows, err = tx.Query(ctx, `SELECT report->>'os',report->>'arch',report->>'version',count(*),count(*) FILTER (WHERE reported_at>now()-interval '2 minutes') FROM device_software GROUP BY 1,2,3 ORDER BY 1,2,3`)
		if err != nil {
			return err
		}
		out.Versions, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.VersionCount, error) {
			var v model.VersionCount
			e := row.Scan(&v.OS, &v.Arch, &v.Version, &v.Devices, &v.Fresh)
			return v, e
		})
		if err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `SELECT device_id,release_id,state,error_code,updated_at FROM update_attempts ORDER BY updated_at DESC LIMIT 100`)
		if err != nil {
			return err
		}
		out.Attempts, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.UpdateAttempt, error) {
			var a model.UpdateAttempt
			e := row.Scan(&a.DeviceID, &a.ReleaseID, &a.State, &a.ErrorCode, &a.UpdatedAt)
			return a, e
		})
		if err != nil {
			return err
		}
		return nil
	})
	return out, err
}
