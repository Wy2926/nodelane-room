package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/model"
)

func TestVersionReportWithoutRoomAndOnGUIChange(t *testing.T) {
	reports := make(chan model.ClientReport, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/auth/challenge":
			_ = json.NewEncoder(w).Encode(model.Challenge{})
		case "/v2/auth/verify":
			_ = json.NewEncoder(w).Encode(model.Session{Token: "test-session", ExpiresAt: time.Now().Add(time.Hour)})
		case "/v2/client/report":
			if r.Header.Get("Authorization") != "Bearer test-session" {
				t.Error("report is not authenticated")
			}
			var report model.ClientReport
			if e := json.NewDecoder(r.Body).Decode(&report); e != nil {
				t.Error(e)
			}
			reports <- report
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/v2/me":
			_ = json.NewEncoder(w).Encode(model.User{ID: "test-user", Name: "player"})
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
	i, e := device.NewIdentity(srv.URL, "player")
	if e != nil {
		t.Fatal(e)
	}
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
