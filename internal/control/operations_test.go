package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/model"
)

func TestRejectedReceiptRollsBackBusinessAndReplaysAcrossReplicas(t *testing.T) {
	s, ca := database(t)
	a := user(t, apiServer(t, s, ca), "alice")
	ctx := context.Background()
	id := randomID()
	deadline := time.Now().UTC().Add(50 * time.Minute)
	ctx = context.WithValue(ctx, operationContextKey{}, operationContext{ID: id, Method: "POST", Path: "/v2/rooms", RequestID: randomID(), Deadline: deadline})
	var original string
	must(t, s.Pool.QueryRow(ctx, "SELECT name FROM games WHERE id='custom'").Scan(&original))
	calls := 0
	fn := func(tx pgx.Tx) (any, error) {
		calls++
		if _, err := tx.Exec(ctx, "UPDATE games SET name='must roll back' WHERE id='custom'"); err != nil {
			return nil, err
		}
		return nil, model.Failure("game_disabled")
	}
	raw, err := s.mutateChecked(ctx, a.Identity.ID(), id, "same-body", nil, fn)
	must(t, err)
	replica := &Store{Pool: s.Pool, Network: s.Network}
	replay, err := replica.mutateChecked(ctx, a.Identity.ID(), id, "same-body", nil, fn)
	must(t, err)
	if string(raw) != string(replay) || calls != 1 {
		t.Fatal("rejection executed twice")
	}
	var name string
	must(t, s.Pool.QueryRow(ctx, "SELECT name FROM games WHERE id='custom'").Scan(&name))
	if name != original {
		t.Fatal("savepoint committed partial mutation")
	}
	var receipt model.Receipt
	must(t, json.Unmarshal(raw, &receipt))
	if receipt.Result.Code != "game_disabled" || receipt.Status != 403 {
		t.Fatal("wrong rejection receipt")
	}
	_, err = s.mutateChecked(ctx, a.Identity.ID(), id, "changed-body", nil, fn)
	if !model.IsCode(err, "request_idempotency_conflict") {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, operationContextKey{}, operationContext{ID: id, Deadline: time.Now().Add(-time.Second)})
	_, err = s.mutateChecked(ctx, a.Identity.ID(), id, "same-body", nil, fn)
	if !model.IsCode(err, "operation_expired") || calls != 1 {
		t.Fatal("expired operation executed", err)
	}
}

func TestReceiptChecksCurrentAuthorityAndSubject(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	a, b := user(t, server, "alice"), user(t, server, "bob")
	ctx := context.Background()
	id := randomID()
	var created model.RoomResult
	must(t, a.Call(client.WithOperation(ctx, id, time.Now().UTC().Add(50*time.Minute)), "POST", "/v2/rooms", model.RoomRequest{Name: "private receipt", Game: "custom", ExpectedGameRevision: 1}, &created))
	join(t, b, created)
	must(t, a.Call(ctx, "POST", "/v2/rooms/"+created.Room.ID+"/transfer", model.MemberRequest{DeviceID: b.Identity.ID(), ExpectedRevision: roomRevision(t, s, created.Room.ID)}, nil))
	var result model.Operation
	must(t, a.Call(ctx, "GET", "/v2/me/operations/"+id, nil, &result))
	if !result.KnownCommit || result.Result == nil || result.Result.Code != "operation_result_redacted" || (len(result.Result.Data) != 0 && string(result.Result.Data) != "null") {
		t.Fatal("old invitation escaped receipt", result.State)
	}
	err := b.Call(ctx, "GET", "/v2/me/operations/"+id, nil, nil)
	if !model.IsCode(err, "operation_not_found") {
		t.Fatal("cross-subject receipt visible", err)
	}
	_, err = s.Pool.Exec(ctx, "UPDATE idempotency SET expires_at=now()-interval '1 second' WHERE device_id=$1 AND key=$2", a.Identity.ID(), id)
	must(t, err)
	err = a.Call(ctx, "GET", "/v2/me/operations/"+id, nil, nil)
	if !model.IsCode(err, "operation_expired") {
		t.Fatal(err)
	}
}

func TestMissingContractIsStructuredAndCapabilitiesArePublic(t *testing.T) {
	handler := (&Server{}).Handler()
	for _, test := range []struct {
		path, code string
		status     int
	}{{"/v2/me", "api_contract_unsupported", 400}, {"/v2/capabilities", "ok", 200}} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, test.path, nil))
		var result model.Result
		if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Contract != model.Contract || result.RequestID == "" || result.Code != test.code || w.Code != test.status {
			t.Fatalf("unexpected envelope: %d %s", w.Code, w.Body.String())
		}
	}
}
