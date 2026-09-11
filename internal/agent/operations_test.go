package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"runtime"
	"testing"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

func TestLedgerPreservesBytesAndRedactsPendingStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires provisioned protected ProgramData directory; exercised in isolated Linux tests")
	}
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r, err := New(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("{ \"code\" : \"temporary-invite\" }")
	c := model.SavedCommand{Operation: model.Operation{ID: "frozen-command-id", State: "submitting", Action: "join", Deadline: time.Now().Add(time.Minute)}, Body: body, Request: []byte(`{"action":"join"}`), RequestHash: "original"}
	r.commands[c.ID] = c
	if err = r.saveCommands(); err != nil {
		t.Fatal(err)
	}
	restored, err := New(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	saved := restored.commands[c.ID]
	if !bytes.Equal(saved.Body, body) || saved.State != "reconciling" {
		t.Fatal("restart lost original intent")
	}
	safe, _ := json.Marshal(restored.Status())
	if bytes.Contains(safe, []byte("temporary-invite")) {
		t.Fatal("status leaked request")
	}
	restored.commandMu.Lock()
	defer restored.commandMu.Unlock()
	done := make(chan struct{})
	go func() { _ = restored.Status(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("status blocked on command ledger")
	}
}

func TestPausePersistsWithoutWaitingForControlOperation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires provisioned protected ProgramData directory; exercised in isolated Linux tests")
	}
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r, err := New(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	r.op.Lock()
	defer r.op.Unlock()
	done := make(chan error, 1)
	go func() { _, e := r.networkAction(context.Background(), "network-stop"); done <- e }()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("pause waited for control")
	}
	restored, err := New(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.paused.Load() {
		t.Fatal("restart discarded pause")
	}
}
