//go:build !windows

package localapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/nodelane/nodelane-room/internal/platform"
)

func TestCallOverLocalSocket(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		err    string
	}{
		{"success", 200, `{"engine":"running"}`, ""},
		{"service error", 400, `{"error":"unsupported node operation"}`, "unsupported node operation"},
		{"plain error", 400, "invalid RPC request\n", "invalid RPC request\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			listener, err := platform.ListenLocal(dir)
			if err != nil {
				t.Fatal(err)
			}
			srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/rpc" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				var req Request
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.Action != "status" || req.Room != "room-id" || string(req.Body) != `{"revision":2}` {
					t.Error("local request fields changed")
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			})}
			done := make(chan struct{})
			go func() { defer close(done); _ = srv.Serve(listener) }()
			t.Cleanup(func() { _ = srv.Close(); <-done })
			var out struct {
				Engine string `json:"engine"`
			}
			err = Call(t.Context(), dir, Request{Action: "status", Room: "room-id", Body: json.RawMessage(`{"revision":2}`)}, &out)
			if test.err != "" {
				if err == nil || err.Error() != test.err {
					t.Fatalf("want %q, got %v", test.err, err)
				}
			} else if err != nil || out.Engine != "running" {
				t.Fatalf("response failed: engine=%q, err=%v", out.Engine, err)
			}
		})
	}
}

func TestCallUnavailable(t *testing.T) {
	err := Call(t.Context(), t.TempDir(), Request{Action: "status"}, nil)
	if err == nil || !strings.Contains(err.Error(), "cannot reach NodeLane service") {
		t.Fatalf("missing service diagnostic: %v", err)
	}
}
