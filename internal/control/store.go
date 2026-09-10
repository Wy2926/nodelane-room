package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

var (
	ErrUnauthorized = errors.New("authentication required")
	ErrForbidden    = errors.New("operation is not permitted")
	ErrNotFound     = errors.New("resource not found")
	ErrConflict     = errors.New("resource conflicts with current state")
	ErrInvalid      = errors.New("invalid request")
	ErrRateLimited  = errors.New("too many requests")
)

type Store struct {
	Pool    *pgxpool.Pool
	Network netip.Prefix
}

func Open(ctx context.Context, url, network string) (*Store, error) {
	n, err := parseNetwork(network)
	if err != nil {
		return nil, err
	}
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return &Store{Pool: p, Network: n}, nil
}

func parseNetwork(network string) (netip.Prefix, error) {
	n, err := netip.ParsePrefix(network)
	if err != nil || !n.Addr().Is4() || n.Bits() < 16 || n.Bits() > 28 || n != n.Masked() {
		return netip.Prefix{}, fmt.Errorf("%w: 地址池须为规范 IPv4 /16 至 /28 网段", ErrInvalid)
	}
	return n, nil
}
func (s *Store) Migrate(ctx context.Context) error {
	return s.Write(ctx, func(tx pgx.Tx) error {
		return s.migrate(ctx, tx)
	})
}

func (s *Store) migrate(ctx context.Context, tx pgx.Tx) error {
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT to_regclass('schema_version') IS NOT NULL").Scan(&exists); err != nil {
		return err
	}
	if exists {
		var valid bool
		if err := tx.QueryRow(ctx, "SELECT count(*)=1 AND min(version)=2 FROM schema_version").Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return fmt.Errorf("database schema is not V2; use an empty database for a new deployment")
		}
	} else {
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema()").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("initialization requires an empty database schema")
		}
	}
	if _, err := tx.Exec(ctx, schema); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO settings(key,value) VALUES('network',$1) ON CONFLICT DO NOTHING", s.Network.String())
	if err != nil {
		return err
	}
	var n string
	if err = tx.QueryRow(ctx, "SELECT value FROM settings WHERE key='network'").Scan(&n); err != nil {
		return err
	}
	if n != s.Network.String() {
		return fmt.Errorf("configured network differs from database: %s", n)
	}
	_, err = tx.Exec(ctx, "INSERT INTO settings(key,value) VALUES('deployment_id',$1) ON CONFLICT DO NOTHING", randomID())
	if err != nil {
		return err
	}
	return nil
}

// All mutations share a database transaction lock. This intentionally favors
// correctness for the bounded room size over write throughput; replicas can serve
// reads concurrently and no correctness decision depends on process-local state.
func (s *Store) Write(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(1313817669)"); err != nil {
		return err
	}
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func hash(s string) string       { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func deviceID(pub []byte) string { h := sha256.Sum256(pub); return hex.EncodeToString(h[:]) }
func noRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Store) allocate(ctx context.Context, tx pgx.Tx, holder string) (string, error) {
	var ip string
	err := tx.QueryRow(ctx, "SELECT ip FROM addresses WHERE holder=$1 AND release_after IS NULL", holder).Scan(&ip)
	if err == nil {
		return ip, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	_, err = tx.Exec(ctx, "DELETE FROM addresses WHERE release_after < now()")
	if err != nil {
		return "", err
	}
	rows, err := tx.Query(ctx, "SELECT ip FROM addresses")
	if err != nil {
		return "", err
	}
	used := map[string]bool{}
	for rows.Next() {
		if err = rows.Scan(&ip); err != nil {
			rows.Close()
			return "", err
		}
		used[ip] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	for a := s.Network.Addr().Next(); s.Network.Contains(a.Next()); a = a.Next() {
		if !used[a.String()] {
			_, err = tx.Exec(ctx, "INSERT INTO addresses(ip,holder) VALUES($1,$2)", a.String(), holder)
			return a.String(), err
		}
	}
	return "", fmt.Errorf("%w: address pool exhausted", ErrConflict)
}
func bump(ctx context.Context, tx pgx.Tx, room, kind string) error {
	var rev int64
	if err := tx.QueryRow(ctx, "UPDATE rooms SET revision=revision+1 WHERE id=$1 RETURNING revision", room).Scan(&rev); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO events(room_id,revision,kind) VALUES($1,$2,$3)", room, rev, kind)
	return err
}
func revoke(ctx context.Context, tx pgx.Tx, room, device string) error {
	if _, err := tx.Exec(ctx, "UPDATE certificates SET revoked=true WHERE room_id=$1 AND ($2='' OR device_id=$2)", room, device); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE addresses SET release_after=now()+interval '11 minutes',holder=holder||':'||$3 WHERE ip IN (SELECT ip FROM members WHERE room_id=$1 AND active AND ($2='' OR device_id=$2))", room, device, randomID()); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE members SET active=false WHERE room_id=$1 AND ($2='' OR device_id=$2)", room, device); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "DELETE FROM endpoints WHERE room_id=$1 AND ($2='' OR device_id=$2)", room, device)
	return err
}
func (s *Store) Sweep(ctx context.Context) error {
	return s.Write(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT id FROM rooms WHERE NOT closed AND expires_at<=now()")
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err = revoke(ctx, tx, id, ""); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "UPDATE rooms SET closed=true WHERE id=$1", id); err != nil {
				return err
			}
			if err = bump(ctx, tx, id, "expired"); err != nil {
				return err
			}
		}
		rows, err = tx.Query(ctx, "DELETE FROM endpoints WHERE expires_at<=now() RETURNING room_id")
		if err != nil {
			return err
		}
		rooms := map[string]bool{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			rooms[id] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for id := range rooms {
			if err = bump(ctx, tx, id, "endpoints"); err != nil {
				return err
			}
		}
		for _, q := range []string{"DELETE FROM challenges WHERE expires_at<now()", "DELETE FROM sessions WHERE expires_at<now()", "DELETE FROM invitations WHERE expires_at<now()", "DELETE FROM enrollment_keys WHERE expires_at<now()-interval '1 day'", "DELETE FROM admin_sessions WHERE expires_at<now() OR last_seen<now()-interval '30 minutes'", "DELETE FROM idempotency WHERE expires_at<now()", "DELETE FROM rate_limits WHERE window_start<now()-interval '1 day'", "DELETE FROM addresses WHERE release_after<now()", "DELETE FROM certificates WHERE expires_at<now()-interval '1 hour'", "DELETE FROM events WHERE created_at<now()-interval '7 days'"} {
			if _, err = tx.Exec(ctx, q); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Store) Rate(ctx context.Context, key string, limit int, window time.Duration) error {
	var count int
	err := s.Pool.QueryRow(ctx, `INSERT INTO rate_limits(key,count,window_start) VALUES($1,1,now()) ON CONFLICT(key) DO UPDATE SET count=CASE WHEN rate_limits.window_start<now()-$2::interval THEN 1 ELSE rate_limits.count+1 END,window_start=CASE WHEN rate_limits.window_start<now()-$2::interval THEN now() ELSE rate_limits.window_start END RETURNING count`, key, fmt.Sprintf("%f seconds", window.Seconds())).Scan(&count)
	if err != nil {
		return err
	}
	if count > limit {
		return ErrRateLimited
	}
	return nil
}

func (s *Store) Mutate(ctx context.Context, device, key, requestHash string, fn func(pgx.Tx) (any, error)) ([]byte, error) {
	return s.mutateChecked(ctx, device, key, requestHash, nil, fn)
}

// Check current authorization inside the same transaction, even for a cached response.
func (s *Store) mutateChecked(ctx context.Context, device, key, requestHash string, check func(pgx.Tx) error, fn func(pgx.Tx) (any, error)) ([]byte, error) {
	if len(key) < 16 || len(key) > 128 {
		return nil, fmt.Errorf("%w: Idempotency-Key must be 16-128 characters", ErrInvalid)
	}
	var response []byte
	err := s.Write(ctx, func(tx pgx.Tx) error {
		if check != nil {
			if err := check(tx); err != nil {
				return err
			}
		}
		var previous string
		err := tx.QueryRow(ctx, "SELECT request_hash,response FROM idempotency WHERE device_id=$1 AND key=$2 AND expires_at>now()", device, key).Scan(&previous, &response)
		if err == nil {
			if previous != requestHash {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		result, err := fn(tx)
		if err != nil {
			return err
		}
		response, err = json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO idempotency(device_id,key,request_hash,response,expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour') ON CONFLICT(device_id,key) DO UPDATE SET request_hash=EXCLUDED.request_hash,response=EXCLUDED.response,expires_at=EXCLUDED.expires_at", device, key, requestHash, response)
		return err
	})
	return response, err
}
