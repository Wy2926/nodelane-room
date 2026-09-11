package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/model"
)

func TestRetryFreezesIntentAndDoesNotInterpretGenericDenials(t *testing.T) {
	var calls atomic.Int32
	raw := json.RawMessage("{ \"name\" : \"original\" }")
	deadline := time.Now().UTC().Add(50 * time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if string(b) != string(raw) || r.Header.Get("Idempotency-Key") != "fixed-operation-123" || r.Header.Get(model.DeadlineHeader) != deadline.Format(time.RFC3339Nano) || r.Header.Get(model.ContractHeader) != model.Contract {
			t.Error("retry changed its frozen request")
		}
		code := "system_unavailable"
		if calls.Add(1) == 3 {
			code = "room_owner_required"
		}
		w.WriteHeader(model.HTTPStatus(code))
		_ = json.NewEncoder(w).Encode(model.NewResult(code, "control", ID(), nil))
	}))
	defer server.Close()
	api := NewAPI(device.Identity{Server: server.URL})
	api.session = model.Session{Token: "test-session", ExpiresAt: time.Now().Add(time.Hour)}
	err := api.Call(WithOperation(context.Background(), "fixed-operation-123", deadline), "POST", "/v2/rooms/example/invite", raw, nil)
	if !model.IsCode(err, "room_owner_required") || calls.Load() != 3 {
		t.Fatalf("calls=%d error=%v", calls.Load(), err)
	}
	if MembershipEnded(err) || IdentityEnded(err) {
		t.Fatal("role denial terminated authorization")
	}
	for _, code := range []string{"resource_not_found", "future_permission_code", "request_state_stale"} {
		if MembershipEnded(model.Failure(code)) || IdentityEnded(model.Failure(code)) {
			t.Fatal("generic denial ended authorization", code)
		}
	}
}

func TestDecodeSnapshotReplacesOmittedAuthorityAndRejectsMalformedData(t *testing.T) {
	snapshot := model.Snapshot{Room: &model.Room{ID: "old"}, Members: []model.Member{{DeviceID: "secret"}}}
	if err := decodeResponse([]byte(`{"server_time":"2026-09-11T00:00:00Z","self":{"room_id":"old","state":"ended","reason":"member_kicked","revision":9}}`), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Room != nil || len(snapshot.Members) != 0 || snapshot.Self.Reason != "member_kicked" {
		t.Fatal("old permissions survived tombstone", snapshot)
	}
	before := snapshot
	if err := decodeResponse([]byte(`{"room":`), &snapshot); err == nil {
		t.Fatal("invalid body accepted")
	}
	if snapshot.Self != before.Self {
		t.Fatal("invalid body partially applied")
	}
	me := model.AccountStatus{User: model.User{ID: "keep"}, Membership: model.MembershipSelf{State: "active", RoomID: "keep-room"}}
	if err := decodeResponse([]byte(`{}`), &me); !model.IsCode(err, "local_control_response_invalid") || me.User.ID != "keep" {
		t.Fatal("missing authority fields replaced current state")
	}
}

func TestOnlyProvenIdentityTerminationStopsAutomaticAuthentication(t *testing.T) {
	for _, tc := range []struct {
		code  string
		calls int32
	}{{"auth_device_revoked", 1}, {"node_revoked", 1}, {"node_generation_stale", 2}, {"room_owner_required", 2}, {"resource_not_found", 2}, {"node_disabled", 2}} {
		t.Run(tc.code, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(model.HTTPStatus(tc.code))
				_ = json.NewEncoder(w).Encode(model.NewResult(tc.code, "control", ID(), nil))
			}))
			defer server.Close()
			api := NewAPI(device.Identity{Server: server.URL})
			api.session = model.Session{Token: "test-session", ExpiresAt: time.Now().Add(time.Hour)}
			for range 2 {
				if err := api.Call(context.Background(), "GET", "/v2/me", nil, nil); !model.IsCode(err, tc.code) {
					t.Fatal(err)
				}
			}
			if calls.Load() != tc.calls {
				t.Fatalf("unexpected automatic calls: %d", calls.Load())
			}
		})
	}
}
