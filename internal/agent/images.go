package agent

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"time"

	"github.com/nodelane/nodelane-room/internal/localapi"
)

// Artwork is public data fetched only from the configured control origin.
// No session, arbitrary URL, redirect, or disk cache is exposed to the WebView.
func (r *Runtime) gameImage(ctx context.Context, game, kind string) ([]byte, string, error) {
	if !validLocalID(game, false) || (kind != "cover" && kind != "background") {
		return nil, "", localapi.Failure("invalid_request", "invalid game image")
	}
	select {
	case r.imageSlots <- struct{}{}:
		defer func() { <-r.imageSlots }()
	default:
		return nil, "", localapi.Failure("busy", "图片请求繁忙，请稍后重试")
	}
	r.stateMu.Lock()
	server := r.status.Server
	r.stateMu.Unlock()
	if server == "" {
		return nil, "", localapi.Failure("unconfigured", "请先初始化设备")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", server+"/v2/games/"+game+"/images/"+kind, nil)
	if err != nil {
		return nil, "", err
	}
	h := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := h.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", localapi.Failure("image_unavailable", "游戏图片暂不可用")
	}
	const limit = 5 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > limit {
		return nil, "", localapi.Failure("image_invalid", "游戏图片过大")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
		return nil, "", localapi.Failure("image_invalid", "游戏图片格式无效")
	}
	return data, "image/" + format, nil
}
