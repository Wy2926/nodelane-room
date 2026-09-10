package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/nodelane/nodelane-room/internal/localapi"
)

type LogBuffer struct {
	sequence uint64
	mu       sync.Mutex
	lines    []localapi.LogEntry
	partial  string
}

func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.partial + string(p)
	parts := strings.Split(s, "\n")
	b.partial = parts[len(parts)-1]
	if len(b.partial) > 8192 {
		b.partial = b.partial[:8192]
	}
	for _, line := range parts[:len(parts)-1] {
		if len(line) > 8192 {
			line = line[:8192]
		}
		b.sequence++
		b.lines = append(b.lines, localapi.LogEntry{Sequence: b.sequence, Line: line})
	}
	if len(b.lines) > 1000 {
		b.lines = append([]localapi.LogEntry(nil), b.lines[len(b.lines)-1000:]...)
	}
	return len(p), nil
}
func (b *LogBuffer) Lines() []localapi.LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]localapi.LogEntry{}, b.lines...)
}
func NodeLogger() (*slog.Logger, *LogBuffer) {
	buffer := &LogBuffer{}
	handler := slog.NewJSONHandler(io.MultiWriter(os.Stderr, buffer), &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		k := strings.ToLower(a.Key)
		for _, secret := range []string{"private", "password", "token", "session", "secret", "pem", "enrollment_key"} {
			if strings.Contains(k, secret) {
				return slog.String(a.Key, "[redacted]")
			}
		}
		if k == "key" || k == "certificate" {
			return slog.String(a.Key, "[redacted]")
		}
		return a
	}})
	return slog.New(handler), buffer
}
func (r *Runtime) SetLogBuffer(b *LogBuffer) { r.logs = b }
func (r *Runtime) nodeLocal(ctx context.Context, in localapi.Request) (any, error) {
	switch in.Action {
	case "status", "config":
		return r.NodeStatus(), nil
	case "enroll":
		var args struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(in.Body, &args); err != nil {
			return nil, errors.New("invalid enrollment request")
		}
		return r.EnrollNode(ctx, args.Key)
	case "apply":
		var args struct {
			Revision int64 `json:"revision"`
		}
		if err := json.Unmarshal(in.Body, &args); err != nil {
			return nil, err
		}
		err := r.ApplyNodeConfig(args.Revision)
		return map[string]bool{"ok": err == nil}, err
	case "restart":
		err := r.RestartNode()
		return map[string]bool{"ok": err == nil}, err
	case "doctor":
		return r.nodeDoctor(ctx), nil
	case "logs":
		if r.logs == nil {
			return []localapi.LogEntry{}, nil
		}
		return r.logs.Lines(), nil
	default:
		return nil, errors.New("unsupported node operation")
	}
}
