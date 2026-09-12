package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/localapi"
	"github.com/nodelane/nodelane-room/internal/model"
)

func TestVersionReportWithoutRoomAndOnGUIChange(t *testing.T) {
	i, e := device.NewIdentity("https://example.test", "player")
	if e != nil {
		t.Fatal(e)
	}
	reports := make(chan model.ClientReport, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/auth/challenge":
			_ = json.NewEncoder(w).Encode(model.NewResult("ok", "control", "trace", model.Challenge{ID: client.ID(), Nonce: make([]byte, 32)}))
		case "/v2/auth/verify":
			_ = json.NewEncoder(w).Encode(model.NewResult("ok", "control", "trace", model.Session{DeviceID: i.ID(), Token: strings.Repeat("t", 64), ExpiresAt: time.Now().Add(time.Hour)}))
		case "/v2/client/report":
			if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 64) {
				t.Error("report is not authenticated")
			}
			var report model.ClientReport
			if e := json.NewDecoder(r.Body).Decode(&report); e != nil {
				t.Error(e)
			}
			reports <- report
			_ = json.NewEncoder(w).Encode(model.NewResult("ok", "control", "trace", map[string]bool{"ok": true}))
		case "/v2/me":
			_ = json.NewEncoder(w).Encode(model.NewResult("ok", "control", "trace", model.AccountStatus{RoomCreation: model.RoomCreationPermission{Reason: "account_disabled"}, User: model.User{ID: "test-user", Name: "player", State: "disabled"}, Device: model.UserDevice{DeviceID: i.ID()}, Membership: model.MembershipSelf{State: "none"}}))
		default:
			t.Error("unexpected request", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	r, e := New(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	i.Server = srv.URL
	r.identity = i
	r.api = client.NewAPI(i)
	for range 2 {
		if e = r.step(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	if len(reports) != 1 {
		t.Fatal("unchanged report was not throttled", len(reports))
	}
	if status := r.Status(); status.Identity != "active" || status.RoomCreation.Allowed || status.RoomCreation.Reason != "account_disabled" {
		t.Fatal("creation restriction was not propagated independently of identity")
	}
	first := <-reports
	if first.Version != model.ClientVersion || first.OS != runtime.GOOS || first.GUIVersion != "" {
		t.Fatalf("wrong report %+v", first)
	}
	r.updateMu.Lock()
	r.guiVersion = model.ClientVersion
	r.updateMu.Unlock()
	if e = r.step(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(reports) != 1 {
		t.Fatal("GUI version change was not reported")
	}
	if got := <-reports; got.GUIVersion != model.ClientVersion {
		t.Fatalf("wrong GUI version %+v", got)
	}
}

func TestUpdateConsentAndCancellation(t *testing.T) {
	r := &Runtime{updateWake: make(chan struct{}, 1), updateState: model.UpdateStatus{State: "available", Release: &model.UpdateRelease{ID: "release1"}}}
	if _, err := r.updateAction(context.Background(), localapi.Request{Action: "update-download", Target: "stale"}); err == nil {
		t.Fatal("accepted a different release")
	}
	if _, err := r.updateAction(context.Background(), localapi.Request{Action: "update-download", Target: "release1"}); err != nil {
		t.Fatal(err)
	}
	if r.updateRequested != "release1" {
		t.Fatal("missing explicit consent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.updateCancel = cancel
	r.updateState.State = "downloading"
	if _, err := r.updateAction(context.Background(), localapi.Request{Action: "update-cancel"}); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil || r.updateRequested != "" {
		t.Fatal("download or installation consent survived cancellation")
	}
	if _, err := r.updateAction(context.Background(), localapi.Request{Action: "update-install"}); err == nil {
		t.Fatal("unverified download was installable")
	}
}

func TestUpdateCancelStopsInFlightRequest(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		close(started)
		<-req.Context().Done()
	}))
	defer srv.Close()
	r := &Runtime{identity: device.Identity{Server: srv.URL}, updateState: model.UpdateStatus{State: "checking"}}
	done := make(chan struct{})
	go func() { defer close(done); r.updateStep(context.Background()) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("check did not start")
	}
	if _, err := r.updateAction(context.Background(), localapi.Request{Action: "update-cancel"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not stop network request")
	}
	if s := r.updateStatus(); s.State != "available" || s.ErrorCode != "" {
		t.Fatalf("cancel reported failure: %+v", s)
	}
}
