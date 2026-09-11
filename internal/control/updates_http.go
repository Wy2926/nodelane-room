package control

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/update"
)

func (s *Server) registerUpdates(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/updates/check", s.publicUpdateCheck)
	mux.HandleFunc("POST /v2/client/report", s.playerMutation(func(r *http.Request, tx pgx.Tx, id string, b []byte) (any, error) {
		var in model.ClientReport
		if err := decodeBytes(b, &in); err != nil {
			return nil, err
		}
		err := reportClient(r.Context(), tx, id, in)
		if err == nil {
			err = enforceUpdates(r.Context(), tx)
		}
		return map[string]bool{"ok": true}, err
	}))
	mux.HandleFunc("GET /v2/admin/updates", s.adminHandler(s.adminUpdates))
	mux.HandleFunc("PUT /v2/admin/updates/sources", s.adminWrite(s.adminMutation(func(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
		var in model.UpdateSource
		if err := decodeBytes(b, &in); err != nil {
			return nil, err
		}
		return saveSource(r.Context(), tx, actor, in)
	})))
	mux.HandleFunc("POST /v2/admin/updates/sources/{source}/test", s.adminWrite(s.adminTestSource))
	mux.HandleFunc("PUT /v2/admin/updates/repository", s.adminWrite(s.adminMutation(func(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
		var in model.UpdateRepository
		if err := decodeBytes(b, &in); err != nil {
			return nil, err
		}
		return saveRepository(r.Context(), tx, actor, in)
	})))
	mux.HandleFunc("PUT /v2/admin/updates/releases", s.adminWrite(s.adminMutation(func(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
		var in model.UpdateRelease
		if err := decodeBytes(b, &in); err != nil {
			return nil, err
		}
		return saveRelease(r.Context(), tx, actor, in)
	})))
	mux.HandleFunc("PUT /v2/admin/updates/policies", s.adminWrite(s.adminMutation(func(r *http.Request, tx pgx.Tx, actor string, b []byte) (any, error) {
		var in model.UpdatePolicy
		if err := decodeBytes(b, &in); err != nil {
			return nil, err
		}
		return savePolicy(r.Context(), tx, actor, in)
	})))
	mux.HandleFunc("PUT /v2/admin/updates/releases/{release}/sources/{source}", s.adminWrite(s.adminReplica))
	mux.HandleFunc("POST /v2/admin/updates/releases/{release}/sources/{source}", s.adminWrite(s.adminReplica))
}

func (s *Server) publicUpdateCheck(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	version, osName, arch := q.Get("version"), q.Get("os"), q.Get("arch")
	if !model.ValidVersion(version) || !model.ValidUpdatePlatform(osName, arch) {
		s.fail(w, ErrInvalid)
		return
	}
	if err := s.Store.Rate(r.Context(), "updates:"+requestIP(r), 120, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	out := model.UpdateCheck{ServerTime: time.Now().UTC(), URLs: []string{}}
	err := s.Store.Write(r.Context(), func(tx pgx.Tx) error {
		if q.Get("installed") == "1" {
			var id string
			if err := tx.QueryRow(r.Context(), "SELECT id FROM update_releases WHERE version=$1 AND os=$2 AND arch=$3 AND state IN ('published','paused')", version, osName, arch).Scan(&id); err != nil {
				return noRows(err)
			}
			release, err := readRelease(r.Context(), tx, id)
			if err != nil {
				return err
			}
			out.Release = &release
			repo, err := readRepository(r.Context(), tx)
			if err != nil {
				return err
			}
			out.Repository = &repo
			out.URLs, err = releaseURLs(r.Context(), tx, release)
			return err
		}
		p, err := readPolicy(r.Context(), tx, osName, arch)
		if err != nil || p == nil {
			return err
		}
		out.Policy = p
		if p.ReleaseID == "" {
			return nil
		}
		out.Required = p.MinimumVersion != "" && p.EffectiveAt != nil && !out.ServerTime.Before(*p.EffectiveAt) && model.CompareVersion(version, p.MinimumVersion) < 0
		release, err := readRelease(r.Context(), tx, p.ReleaseID)
		if err != nil {
			return err
		}
		if release.State != "published" || model.CompareVersion(version, release.Version) >= 0 {
			return nil
		}
		out.Release = &release
		out.Repository = &model.UpdateRepository{}
		*out.Repository, err = readRepository(r.Context(), tx)
		if err != nil {
			return err
		}
		out.URLs, err = releaseURLs(r.Context(), tx, release)
		return err
	})
	s.result(w, out, err)
}

func (s *Server) adminUpdates(w http.ResponseWriter, r *http.Request, _ string) {
	out := model.UpdateOverview{Sources: []model.UpdateSource{}, Releases: []model.UpdateRelease{}, Policies: []model.UpdatePolicy{}, Devices: []model.UpdateDevice{}}
	after := r.URL.Query().Get("after")
	if after != "" && !validResourceID(after) && len(after) != 64 {
		s.fail(w, ErrInvalid)
		return
	}
	err := s.Store.Write(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
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
	s.result(w, out, err)
}

func (s *Server) adminTestSource(w http.ResponseWriter, r *http.Request, actor string) {
	var src model.UpdateSource
	err := s.Store.Write(r.Context(), func(tx pgx.Tx) error {
		var e error
		src, e = readSource(r.Context(), tx, r.PathValue("source"), true)
		return e
	})
	if err == nil {
		if src.Kind == "https" {
			var req *http.Request
			req, err = http.NewRequestWithContext(r.Context(), "HEAD", src.PublicURL, nil)
			if err == nil {
				var res *http.Response
				res, err = update.HTTPClient().Do(req)
				if err == nil {
					res.Body.Close()
					if res.StatusCode < 200 || res.StatusCode >= 400 {
						err = ErrConflict
					}
				}
			}
		} else {
			_, err = sourceClient(src).HeadBucket(r.Context(), &s3.HeadBucketInput{Bucket: aws.String(src.Bucket)})
		}
		if err != nil {
			err = errors.New("update source connection failed; check endpoint and permissions")
		}
	}
	if err == nil {
		err = s.Store.Write(r.Context(), func(tx pgx.Tx) error { return adminEvent(r.Context(), tx, actor, "update.source_tested", src.ID, nil) })
	}
	s.result(w, map[string]bool{"ok": true}, err)
}

func (s *Server) adminReplica(w http.ResponseWriter, r *http.Request, actor string) {
	if s.updateTransfers.Add(1) > 2 {
		s.updateTransfers.Add(-1)
		s.fail(w, ErrRateLimited)
		return
	}
	defer s.updateTransfers.Add(-1)
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(40 * time.Minute))
	_ = controller.SetWriteDeadline(time.Now().Add(40 * time.Minute))
	var src model.UpdateSource
	var release model.UpdateRelease
	ctx := r.Context()
	err := s.Store.Write(ctx, func(tx pgx.Tx) error {
		var e error
		src, e = readSource(ctx, tx, r.PathValue("source"), true)
		if e != nil {
			return e
		}
		release, e = readRelease(ctx, tx, r.PathValue("release"))
		return e
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	if release.State == "withdrawn" {
		s.fail(w, ErrConflict)
		return
	}
	if r.Method == "PUT" {
		if src.Kind == "https" {
			s.fail(w, ErrInvalid)
			return
		}
		f, e := os.CreateTemp("", "nlroom-upload-*")
		if e != nil {
			s.fail(w, e)
			return
		}
		defer os.Remove(f.Name())
		defer f.Close()
		n, e := io.Copy(f, http.MaxBytesReader(w, r.Body, release.Size+1))
		if e != nil || n != release.Size {
			s.fail(w, ErrInvalid)
			return
		}
		if e = update.VerifyFile(f.Name(), release.UpdateArtifact); e != nil {
			s.fail(w, ErrInvalid)
			return
		}
		if _, e = f.Seek(0, io.SeekStart); e != nil {
			s.fail(w, e)
			return
		}
		_, err = sourceClient(src).PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(src.Bucket), Key: aws.String(update.ObjectKey(src.Prefix, release.Target)), Body: f, ContentLength: aws.Int64(release.Size), ContentType: aws.String("application/octet-stream"), IfNoneMatch: aws.String("*")})
		// An existing object is accepted only when its complete signed digest matches.
		if err != nil {
			err = verifyReplica(ctx, src, release.UpdateArtifact)
		}
	}
	if err == nil {
		err = verifyReplica(ctx, src, release.UpdateArtifact)
	}
	if err == nil {
		err = s.Store.Write(ctx, func(tx pgx.Tx) error {
			cookie, _ := r.Cookie(adminCookie)
			if cookie == nil {
				return ErrUnauthorized
			}
			if e := validAdminSession(ctx, tx, hash(cookie.Value)); e != nil {
				return e
			}
			current, e := readSource(ctx, tx, src.ID, false)
			if e != nil {
				return e
			}
			latest, e := readRelease(ctx, tx, release.ID)
			if e != nil {
				return e
			}
			if current.Revision != src.Revision || latest.Revision != release.Revision {
				return ErrConflict
			}
			_, e = tx.Exec(ctx, `INSERT INTO update_replicas(release_id,source_id,verified_at,source_revision) VALUES($1,$2,now(),$3) ON CONFLICT(release_id,source_id) DO UPDATE SET verified_at=now(),source_revision=$3`, release.ID, src.ID, src.Revision)
			if e == nil {
				e = adminEvent(ctx, tx, actor, "update.replica_verified", release.ID, map[string]string{"source_id": src.ID})
			}
			return e
		})
	}
	s.result(w, map[string]bool{"ok": true}, err)
}
