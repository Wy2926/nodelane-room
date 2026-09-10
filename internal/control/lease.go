package control

import (
	"context"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
)

func (s *Store) issueLease(ctx context.Context, tx pgx.Tx, ca *pki.Authority, room, device string, in model.LeaseRequest) (model.Lease, error) {
	var out model.Lease
	if len(in.PublicKey) != 32 {
		return out, ErrInvalid
	}
	until := time.Now().UTC().Add(model.LeaseDuration)
	groups := []string{}
	if room != "" {
		if err := activeMember(ctx, tx, room, device); err != nil {
			return out, err
		}
		var expires time.Time
		err := tx.QueryRow(ctx, "SELECT m.ip,r.expires_at FROM members m JOIN rooms r ON r.id=m.room_id WHERE m.room_id=$1 AND m.device_id=$2 AND m.active", room, device).Scan(&out.IP, &expires)
		if err != nil {
			return out, err
		}
		if expires.Before(until) {
			until = expires
		}
		groups = []string{"room:" + room}
		out.RoomID = room
		if _, err = tx.Exec(ctx, "UPDATE members SET last_seen=now() WHERE room_id=$1 AND device_id=$2", room, device); err != nil {
			return out, err
		}
	} else {
		n, err := nodeForDevice(ctx, tx, device)
		if err != nil || (n.State != "active" && n.State != "draining") {
			return out, ErrForbidden
		}
		if in.Revision > 0 {
			var c model.NodeConfig
			if in.Revision > n.Revision {
				return out, ErrInvalid
			}
			if err = tx.QueryRow(ctx, "SELECT config FROM node_configs WHERE node_id=$1 AND revision=$2", n.ID, in.Revision).Scan(&c); err != nil {
				return out, ErrInvalid
			}
			n.Name = c.Name
			n.Region = c.Region
			n.Address = c.Address
			n.Lighthouse = c.Lighthouse
			n.Relay = c.Relay
			n.Notes = c.Notes
			n.Revision = in.Revision
		}

		out.Node = &n
		out.IP = n.IP
		groups = []string{"infrastructure"}

	}
	ip, err := netip.ParseAddr(out.IP)
	if err != nil {
		return out, err
	}
	c, err := ca.Sign(device, netip.PrefixFrom(ip, s.Network.Bits()), groups, in.PublicKey, until)
	if err != nil {
		return out, err
	}
	pem, err := c.MarshalPEM()
	if err != nil {
		return out, err
	}
	f, err := c.Fingerprint()
	if err != nil {
		return out, err
	}
	out.Network = s.Network.String()
	out.CA = ca.PEM
	out.Certificate = string(pem)
	out.ExpiresAt = c.NotAfter()
	out.Fingerprint = f
	_, err = tx.Exec(ctx, "INSERT INTO certificates(fingerprint,device_id,room_id,ip,public_key,expires_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING", f, device, room, out.IP, in.PublicKey, out.ExpiresAt)
	return out, err
}

func (s *Store) nodeLease(ctx context.Context, ca *pki.Authority, device, key, requestHash string, in model.LeaseRequest) ([]byte, error) {
	return s.mutateChecked(ctx, device, key, requestHash, func(tx pgx.Tx) error {
		n, err := nodeForDevice(ctx, tx, device)
		if err != nil || (n.State != "active" && n.State != "draining") {
			return ErrForbidden
		}
		var obsolete bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM idempotency i LEFT JOIN certificates c ON c.fingerprint=i.response->>'fingerprint' WHERE i.device_id=$1 AND i.key=$2 AND (c.fingerprint IS NULL OR c.revoked OR c.expires_at<=now()))`, device, key).Scan(&obsolete)
		if err != nil {
			return err
		}
		if obsolete {
			return ErrConflict
		}
		return nil
	}, func(tx pgx.Tx) (any, error) { return s.issueLease(ctx, tx, ca, "", device, in) })
}
