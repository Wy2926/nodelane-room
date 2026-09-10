package control

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/argon2"
)

const adminCookie = "__Host-nlroom-admin"

func passwordHash(password string) (string, error) {
	if len(password) < 12 || len(password) > 128 {
		return "", fmt.Errorf("%w: password must contain 12–128 bytes", ErrInvalid)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32)
	return "argon2id$v=19$m=65536,t=3,p=1$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(sum), nil
}
func passwordOK(encoded, password string) bool {
	if len(password) > 128 {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" || parts[2] != "m=65536,t=3,p=1" {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[3])
	sum, se := base64.RawStdEncoding.DecodeString(parts[4])
	if e != nil || se != nil || len(salt) != 16 || len(sum) != 32 {
		return false
	}
	return subtle.ConstantTimeCompare(sum, argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32)) == 1
}
func (s *Store) ResetAdminPassword(ctx context.Context, password string) error {
	encoded, err := passwordHash(password)
	if err != nil {
		return err
	}
	return s.Write(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE administrator SET password_hash=$1 WHERE id=1", encoded)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrNotFound
		}
		if _, err = tx.Exec(ctx, "DELETE FROM admin_sessions"); err != nil {
			return err
		}
		return adminEvent(ctx, tx, "local-console", "admin.password_reset", "administrator", struct{}{})
	})
}

type adminCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Store) loginAdmin(ctx context.Context, in adminCredentials) (string, error) {
	var encoded string
	if err := s.Pool.QueryRow(ctx, "SELECT password_hash FROM administrator WHERE username=$1", in.Username).Scan(&encoded); err != nil || !passwordOK(encoded, in.Password) {
		return "", ErrUnauthorized
	}
	token := randomID() + randomID()
	csrf := hash("admin-csrf:" + token)
	err := s.Write(ctx, func(tx pgx.Tx) error {
		// A password reset concurrent with login must invalidate this attempt too.
		var current string
		if err := tx.QueryRow(ctx, "SELECT password_hash FROM administrator WHERE username=$1", in.Username).Scan(&current); err != nil {
			return err
		}
		if current != encoded {
			return ErrUnauthorized
		}
		if _, err := tx.Exec(ctx, "INSERT INTO admin_sessions(token_hash,csrf_hash,expires_at) VALUES($1,$2,now()+interval '12 hours')", hash(token), hash(csrf)); err != nil {
			return err
		}
		return adminEvent(ctx, tx, in.Username, "admin.login", "administrator", struct{}{})
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) logoutAdmin(ctx context.Context, tokenHash string) error {
	_, err := s.Pool.Exec(ctx, "DELETE FROM admin_sessions WHERE token_hash=$1", tokenHash)
	return err
}

func (s *Store) changeAdminPassword(ctx context.Context, actor, tokenHash, current, password string) error {
	var encoded string
	if err := s.Pool.QueryRow(ctx, "SELECT password_hash FROM administrator WHERE id=1").Scan(&encoded); err != nil || !passwordOK(encoded, current) {
		return ErrUnauthorized
	}
	next, err := passwordHash(password)
	if err != nil {
		return err
	}
	return s.Write(ctx, func(tx pgx.Tx) error {
		if err := validAdminSession(ctx, tx, tokenHash); err != nil {
			return err
		}
		tag, e := tx.Exec(ctx, "UPDATE administrator SET password_hash=$1 WHERE id=1 AND password_hash=$2", next, encoded)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		if _, e = tx.Exec(ctx, "DELETE FROM admin_sessions"); e != nil {
			return e
		}
		return adminEvent(ctx, tx, actor, "admin.password_changed", "administrator", struct{}{})
	})
}

func validAdminSession(ctx context.Context, tx pgx.Tx, tokenHash string) error {
	var valid bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM admin_sessions WHERE token_hash=$1 AND expires_at>now() AND last_seen>now()-interval '30 minutes')", tokenHash).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrUnauthorized
	}
	return nil
}
