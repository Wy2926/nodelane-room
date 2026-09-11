// Package localapi defines the local service protocol and its client.
package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"

	"crypto/rand"
	"encoding/hex"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
)

func Call(ctx context.Context, dir string, in Request, out any) error {
	in.Contract = model.Contract
	if in.CommandID == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		in.CommandID = hex.EncodeToString(b)
	}
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
		return Failure("local_service_unavailable", "本机服务不可用")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 4<<20 {
		return Failure("local_ipc_response_invalid", "本机响应无效")
	}
	var result model.Result
	if json.Unmarshal(data, &result) != nil || result.Contract != model.Contract || result.RequestID == "" {
		return Failure("local_ipc_response_invalid", "本机响应无效")
	}
	if envelope, ok := out.(*model.Result); ok {
		*envelope = result
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &Error{Code: result.Code, Message: result.Message, Result: &result}
	}
	if _, ok := out.(*model.Result); ok {
		return nil
	}
	if out != nil {
		return json.Unmarshal(result.Data, out)
	}
	return nil
}
