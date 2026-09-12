package control

import (
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

// InvitationPreview reveals only the room summary to a holder of a live invitation.
type InvitationPreview struct {
	Name            string    `json:"name"`
	GameName        string    `json:"game_name"`
	MemberCount     int       `json:"member_count"`
	Capacity        int       `json:"capacity"`
	RoomExpiresAt   time.Time `json:"room_expires_at"`
	InviteExpiresAt time.Time `json:"invite_expires_at"`
}

var invitationCode = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (s *Server) invitationPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	if err := s.Store.Rate(r.Context(), "invite-preview:"+requestIP(r), 240, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	var in model.JoinRequest
	if !jsonContentType(r) {
		s.fail(w, model.Failure("request_media_unsupported"))
		return
	}
	if err := decodeLimitedRequest(w, r, &in, 256); err != nil {
		s.fail(w, err)
		return
	}
	if !invitationCode.MatchString(in.Code) {
		s.fail(w, model.Failure("invite_unusable"))
		return
	}
	var out InvitationPreview
	err := s.Store.Pool.QueryRow(r.Context(), `SELECT r.name,g.name,
		(SELECT count(*) FROM members m JOIN user_devices d ON d.device_id=m.device_id JOIN users u ON u.id=m.user_id
		 WHERE m.room_id=r.id AND m.active AND NOT d.revoked AND (d.expires_at IS NULL OR d.expires_at>now()) AND u.state IN ('active','disabled')),
		r.capacity,r.expires_at,i.expires_at
		FROM invitations i JOIN rooms r ON r.id=i.room_id JOIN games g ON g.id=r.game
		WHERE i.code_hash=$1 AND i.expires_at>now() AND r.expires_at>now() AND NOT r.closed AND g.enabled`, hash(in.Code)).Scan(
		&out.Name, &out.GameName, &out.MemberCount, &out.Capacity, &out.RoomExpiresAt, &out.InviteExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = model.Failure("invite_unusable")
	}
	s.result(w, out, err)
}
