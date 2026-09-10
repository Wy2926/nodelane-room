// Package localapi defines the local service protocol and its client.
package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/nodelane/nodelane-room/internal/platform"
)

func Call(ctx context.Context, dir string, in Request, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return platform.DialLocal(ctx, dir) }}
	defer transport.CloseIdleConnections()
	h := &http.Client{Transport: transport, Timeout: 45 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://nodelane/rpc", bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp, err := h.Do(req)
	if err != nil {
		return errors.New("cannot reach NodeLane service; install/start it or run nlroom-service daemon: " + err.Error())
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		var e Error
		if json.Unmarshal(data, &e) != nil || e.Message == "" {
			return errors.New(string(data))
		}
		return &e
	}
	if len(data) > 4<<20 {
		return Failure("response_too_large", "local response exceeds limit")
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}
