package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nodelane/nodelane-room/internal/model"
)

func previewInvitation(t *testing.T, handler http.Handler, code string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(model.JoinRequest{Code: code})
	must(t, err)
	r := contractRequest(http.MethodPost, "/v2/invitations/preview", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestInvitationPreviewAnonymousSummaryIsReadOnly(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	owner, guest := user(t, server, "private-owner"), user(t, server, "private-guest")
	room := create(t, owner)
	join(t, guest, room)
	handler := (&Server{Store: s, CA: ca}).Handler()
	ctx := context.Background()
	revision := roomRevision(t, s, room.Room.ID)
	var members, invitations int
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM members WHERE room_id=$1", room.Room.ID).Scan(&members))
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM invitations WHERE room_id=$1", room.Room.ID).Scan(&invitations))
	for range 2 {
		w := previewInvitation(t, handler, room.Invitation.Code)
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Set-Cookie") != "" {
			t.Fatalf("anonymous preview has incorrect status or privacy headers: %d", w.Code)
		}
		data := responseData(t, w.Body.Bytes())
		var summary InvitationPreview
		must(t, json.Unmarshal(data, &summary))
		var gameName string
		must(t, s.Pool.QueryRow(ctx, "SELECT name FROM games WHERE id=$1", room.Room.Game).Scan(&gameName))
		if summary.Name != room.Room.Name || summary.GameName != gameName || summary.MemberCount != 2 || summary.Capacity != room.Room.Capacity || !summary.RoomExpiresAt.Equal(room.Room.ExpiresAt) || !summary.InviteExpiresAt.Equal(room.Invitation.ExpiresAt) {
			t.Fatal("preview does not match the room summary")
		}
		var fields map[string]json.RawMessage
		must(t, json.Unmarshal(data, &fields))
		allowed := []string{"name", "game_name", "member_count", "capacity", "room_expires_at", "invite_expires_at"}
		if len(fields) != len(allowed) {
			t.Fatal("preview exposes fields outside the public summary")
		}
		for _, key := range allowed {
			if _, ok := fields[key]; !ok {
				t.Fatalf("preview is missing public field %s", key)
			}
		}
		for _, secret := range []string{room.Invitation.Code, room.Room.ID, owner.Identity.ID(), guest.Identity.ID(), "private-owner", "private-guest"} {
			if bytes.Contains(w.Body.Bytes(), []byte(secret)) {
				t.Fatal("preview exposed an invitation, identity, or private identifier")
			}
		}
	}
	var afterMembers, afterInvitations int
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM members WHERE room_id=$1", room.Room.ID).Scan(&afterMembers))
	must(t, s.Pool.QueryRow(ctx, "SELECT count(*) FROM invitations WHERE room_id=$1", room.Room.ID).Scan(&afterInvitations))
	if roomRevision(t, s, room.Room.ID) != revision || members != afterMembers || invitations != afterInvitations {
		t.Fatal("preview mutated room membership, invitations, or revision")
	}
}

func TestInvitationPreviewRejectsUnavailableRoomsAndInvitations(t *testing.T) {
	for _, reason := range []string{"closed_room", "expired_room", "revoked_invite", "rotated_invite", "expired_invite", "disabled_game", "unknown_invite"} {
		t.Run(reason, func(t *testing.T) {
			s, ca := database(t)
			owner := user(t, apiServer(t, s, ca), "owner")
			room := create(t, owner)
			ctx := context.Background()
			code := room.Invitation.Code
			var query string
			switch reason {
			case "closed_room":
				query = "UPDATE rooms SET closed=true WHERE id=$1"
			case "expired_room":
				query = "UPDATE rooms SET expires_at=now()-interval '1 second' WHERE id=$1"
			case "revoked_invite":
				query = "DELETE FROM invitations WHERE room_id=$1"
			case "expired_invite":
				query = "UPDATE invitations SET expires_at=now()-interval '1 second' WHERE room_id=$1"
			case "disabled_game":
				query = "UPDATE games SET enabled=false WHERE id=(SELECT game FROM rooms WHERE id=$1)"
			case "unknown_invite":
				code = strings.Repeat("0", 32)
			case "rotated_invite":
				var replacement model.Invitation
				must(t, owner.Call(ctx, http.MethodPost, "/v2/rooms/"+room.Room.ID+"/invite", model.MemberRequest{ExpectedRevision: roomRevision(t, s, room.Room.ID)}, &replacement))
				w := previewInvitation(t, (&Server{Store: s, CA: ca}).Handler(), replacement.Code)
				if w.Code != http.StatusOK {
					t.Fatalf("replacement invitation was not usable: %d", w.Code)
				}
			}
			if query != "" {
				_, err := s.Pool.Exec(ctx, query, room.Room.ID)
				must(t, err)
			}
			w := previewInvitation(t, (&Server{Store: s, CA: ca}).Handler(), code)
			var result model.Result
			must(t, json.Unmarshal(w.Body.Bytes(), &result))
			if w.Code != http.StatusForbidden || result.Code != "invite_unusable" || len(result.Data) > 0 && string(result.Data) != "null" {
				t.Fatalf("unavailable invitation did not return the common rejection: status %d, code %s", w.Code, result.Code)
			}
		})
	}
}

func TestInvitationPreviewValidatesRequestBeforeLookup(t *testing.T) {
	s, ca := database(t)
	handler := (&Server{Store: s, CA: ca}).Handler()
	for _, test := range []struct{ name, media, body, code string }{
		{"missing_media", "", `{}`, "request_media_unsupported"},
		{"wrong_media", "text/plain", `{}`, "request_media_unsupported"},
		{"media_prefix", "application/json-invalid", `{}`, "request_media_unsupported"},
		{"empty_body", "application/json", "", "request_malformed"},
		{"unknown_field", "application/json", `{"extra":true}`, "request_malformed"},
		{"trailing_document", "application/json", `{} {}`, "request_malformed"},
		{"wrong_type", "application/json", `{"code":123}`, "request_malformed"},
		{"oversize", "application/json", strings.Repeat(" ", 257), "request_too_large"},
		{"missing_code", "application/json", `{}`, "invite_unusable"},
		{"invalid_code", "application/json", `{"code":"short"}`, "invite_unusable"},
		{"uppercase_code", "application/json", `{"code":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`, "invite_unusable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := contractRequest(http.MethodPost, "/v2/invitations/preview", strings.NewReader(test.body))
			r.Header.Set("Content-Type", test.media)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			var result model.Result
			must(t, json.Unmarshal(w.Body.Bytes(), &result))
			if w.Code != model.HTTPStatus(test.code) || result.Code != test.code {
				t.Fatalf("request rejected incorrectly: status %d, code %s", w.Code, result.Code)
			}
		})
	}
}

func TestInvitationPagePrivacyLanguagesAndExactRoutes(t *testing.T) {
	handler := (&Server{AdminPath: "/private-invitation-test-entry"}).Handler()
	for _, page := range []struct{ path, lang, other, download string }{{"/join", "zh-CN", "/en/join", "/download"}, {"/en/join", "en", "/join", "/en/download"}} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, page.path, nil))
		body := w.Body.String()
		if w.Code != http.StatusOK || w.Header().Get("Content-Language") != page.lang || w.Header().Get("Referrer-Policy") != "no-referrer" || !strings.Contains(w.Header().Get("X-Robots-Tag"), "noindex") || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Content-Security-Policy") == "" {
			t.Fatalf("invitation page privacy or language headers are incorrect: %s", page.path)
		}
		for _, required := range []string{`<html lang="` + page.lang + `"`, `href="` + page.other + `"`, `href="` + page.download + `"`, "data-invite-open", "data-invite-room", "/site-assets/invitation.js"} {
			if !strings.Contains(body, required) {
				t.Fatalf("invitation page missing language, room, open, or download control: %s", page.path)
			}
		}
		if strings.Contains(body, "/private-invitation-test-entry") {
			t.Fatal("invitation page exposes the private admin entry")
		}
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodHead, page.path, nil))
		if w.Code != http.StatusOK || w.Body.Len() != 0 {
			t.Fatal("invitation HEAD response must have no body")
		}
	}
	for _, path := range []string{"/join/unknown", "/en/join/unknown", "/siteweb/join.html", "/siteweb/en/join.html"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("unregistered invitation path was served: %s", path)
		}
	}
}
