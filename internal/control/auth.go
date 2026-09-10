package control

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func (s *Store) Challenge(ctx context.Context, in model.ChallengeRequest) (model.Challenge, error) {
	return s.challenge(ctx, in, "player", "")
}
func (s *Store) challenge(ctx context.Context, in model.ChallengeRequest, scope, key string) (model.Challenge, error) {
	var out model.Challenge
	if len(in.PublicKey) != ed25519.PublicKeySize || !model.ValidLabel(in.Name, 80) || in.DeviceID != deviceID(in.PublicKey) {
		return out, ErrInvalid
	}
	out.ID = randomID()
	out.Nonce = make([]byte, 32)
	if _, err := rand.Read(out.Nonce); err != nil {
		return out, err
	}
	err := s.Write(ctx, func(tx pgx.Tx) error {
		var grant *string
		if scope == "enrollment" {
			var id string
			if len(key) != 64 {
				return ErrForbidden
			}
			if err := tx.QueryRow(ctx, `SELECT k.id FROM enrollment_keys k JOIN nodes n ON n.id=k.node_id WHERE k.key_hash=$1 AND NOT k.revoked AND k.consumed_by IS NULL AND k.expires_at>now() AND k.generation=n.generation AND k.revision=n.revision AND n.state='pending'`, hash(key)).Scan(&id); err != nil {
				return ErrForbidden
			}
			grant = &id
			var used bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM devices WHERE id=$1)", in.DeviceID).Scan(&used); err != nil {
				return err
			}
			if used {
				return ErrConflict
			}
		} else if scope == "node" {
			if _, err := nodeForDevice(ctx, tx, in.DeviceID); err != nil {
				return ErrForbidden
			}
		} else {
			var used bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM node_bindings WHERE device_id=$1)", in.DeviceID).Scan(&used); err != nil {
				return err
			}
			if used {
				return ErrForbidden
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO challenges(id,device_id,name,public_key,nonce,scope,grant_id,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,now()+interval '1 minute')`, out.ID, in.DeviceID, in.Name, in.PublicKey, out.Nonce, scope, grant)
		return err
	})
	return out, err
}
func (s *Store) Verify(ctx context.Context, in model.VerifyRequest) (model.Session, error) {
	return s.verify(ctx, in, "player")
}
func (s *Store) verify(ctx context.Context, in model.VerifyRequest, scope string) (model.Session, error) {
	var out model.Session
	var id, name string
	var pub, nonce []byte
	var grant *string
	err := s.Pool.QueryRow(ctx, `DELETE FROM challenges WHERE id=$1 AND scope=$2 AND expires_at>now() RETURNING device_id,name,public_key,nonce,grant_id`, in.ID, scope).Scan(&id, &name, &pub, &nonce, &grant)
	if err != nil {
		return out, ErrUnauthorized
	}
	message := append([]byte("nodelane-auth-v2:"+scope+":"+in.ID+":"), nonce...)
	if !ed25519.Verify(pub, message, in.Signature) {
		return out, ErrUnauthorized
	}
	sessionScope := scope
	if scope == "enrollment" {
		sessionScope = "node"
	}
	out = model.Session{Token: randomID() + randomID(), DeviceID: id, ExpiresAt: time.Now().UTC().Add(time.Hour)}
	err = s.Write(ctx, func(tx pgx.Tx) error {
		if scope == "enrollment" {
			if grant == nil {
				return ErrForbidden
			}
			if err := s.completeEnrollment(ctx, tx, *grant, id, name, pub); err != nil {
				return err
			}
		} else if scope == "node" {
			if _, err := nodeForDevice(ctx, tx, id); err != nil {
				return ErrForbidden
			}
		} else {
			var used bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM node_bindings WHERE device_id=$1)", id).Scan(&used); err != nil {
				return err
			}
			if used {
				return ErrForbidden
			}
			if _, err := tx.Exec(ctx, "INSERT INTO devices(id,name,public_key) VALUES($1,$2,$3) ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name", id, name, pub); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, "INSERT INTO sessions(token_hash,device_id,scope,expires_at) VALUES($1,$2,$3,$4)", hash(out.Token), id, sessionScope, out.ExpiresAt)
		return err
	})
	return out, err
}
func (s *Store) Authenticate(ctx context.Context, token string) (string, error) {
	return s.authenticate(ctx, token, "player")
}
func (s *Store) authenticate(ctx context.Context, token, scope string) (string, error) {
	if len(token) != 64 {
		return "", ErrUnauthorized
	}
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT device_id FROM sessions WHERE token_hash=$1 AND scope=$2 AND expires_at>now() AND ($2<>'node' OR EXISTS(SELECT 1 FROM node_bindings b JOIN nodes n ON n.id=b.node_id WHERE b.device_id=sessions.device_id AND b.revoked_at IS NULL AND n.state<>'revoked' AND b.generation=n.generation))`, hash(token), scope).Scan(&id)
	if err != nil {
		return "", ErrUnauthorized
	}
	return id, nil
}
func activeMember(ctx context.Context, tx pgx.Tx, room, device string) error {
	var ok bool
	err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM members m JOIN rooms r ON r.id=m.room_id WHERE m.room_id=$1 AND m.device_id=$2 AND m.active AND NOT r.closed AND r.expires_at>now())", room, device).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
func owner(ctx context.Context, tx pgx.Tx, room, device string) error {
	var ok bool
	err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM rooms WHERE id=$1 AND owner_id=$2 AND NOT closed AND expires_at>now())", room, device).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: active room owner required", ErrForbidden)
	}
	return nil
}
