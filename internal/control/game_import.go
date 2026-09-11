package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

type gameImageData struct {
	Data        []byte
	ContentType string
}
type importedGame struct {
	Game   model.Game
	Images map[string]gameImageData
}

func steamAppID(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host != "store.steampowered.com" || u.User != nil || len(raw) > 2048 {
		return "", fmt.Errorf("%w: 请粘贴 https://store.steampowered.com/app/数字/ 游戏链接", ErrInvalid)
	}
	p := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(p) < 2 || len(p) > 3 || p[0] != "app" {
		return "", ErrInvalid
	}
	id, err := strconv.ParseUint(p[1], 10, 32)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != p[1] {
		return "", ErrInvalid
	}
	return p[1], nil
}

func allowedSteamURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 4096 || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return false
	}
	host := u.Hostname()
	return host == "store.steampowered.com" || host == "steamstatic.com" || strings.HasSuffix(host, ".steamstatic.com")
}

func steamHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Resolve and dial the checked IP ourselves, including after redirects.
	// No ambient proxy can redirect this bounded importer into a private host.
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port != "443" || !allowedSteamURL("https://"+host) {
			return nil, fmt.Errorf("Steam address rejected")
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !publicGameIP(ip) {
				return nil, fmt.Errorf("Steam address rejected")
			}
		}
		var last error
		for _, ip := range ips {
			conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		if last == nil {
			last = fmt.Errorf("Steam address unavailable")
		}
		return nil, last
	}
	return &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !allowedSteamURL(req.URL.String()) {
			return fmt.Errorf("Steam redirect rejected")
		}
		return nil
	}}
}

func publicGameIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() &&
		!netip.MustParsePrefix("100.64.0.0/10").Contains(ip) && !netip.MustParsePrefix("198.18.0.0/15").Contains(ip)
}

func fetchSteam(ctx context.Context, c *http.Client, raw string, limit int64) ([]byte, error) {
	if !allowedSteamURL(raw) {
		return nil, fmt.Errorf("%w: Steam 返回了不支持的资源地址", ErrInvalid)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "NodeLane-Room/"+model.ControlVersion)
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: 无法下载 Steam 资料或图片，请稍后重试", ErrInvalid)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%w: Steam 资源暂不可用（HTTP %d）", ErrInvalid, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, fmt.Errorf("%w: Steam 资源过大或下载不完整", ErrInvalid)
	}
	return b, nil
}

var gameHTMLTags = regexp.MustCompile(`<[^>]*>`)

func importSteamGame(ctx context.Context, id string, c *http.Client) (importedGame, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	defer c.CloseIdleConnections()
	out := importedGame{Images: map[string]gameImageData{}}
	b, err := fetchSteam(ctx, c, "https://store.steampowered.com/api/appdetails?appids="+id+"&l=schinese", 2<<20)
	if err != nil {
		return out, err
	}
	var response map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Type          string `json:"type"`
			AppID         uint32 `json:"steam_appid"`
			Name          string `json:"name"`
			Summary       string `json:"short_description"`
			Cover         string `json:"header_image"`
			Background    string `json:"background"`
			BackgroundRaw string `json:"background_raw"`
		} `json:"data"`
	}
	if err = json.Unmarshal(b, &response); err != nil {
		return out, fmt.Errorf("%w: Steam 资料格式无法识别", ErrInvalid)
	}
	app := response[id]
	d := app.Data
	if !app.Success || d.Type != "game" || strconv.FormatUint(uint64(d.AppID), 10) != id || !model.ValidLabel(d.Name, 200) {
		return out, fmt.Errorf("%w: Steam 未返回有效游戏，请检查链接或地区可用性", ErrInvalid)
	}
	summary := strings.Join(strings.Fields(html.UnescapeString(gameHTMLTags.ReplaceAllString(d.Summary, " "))), " ")
	if len(summary) > 4000 {
		return out, fmt.Errorf("%w: 游戏简介过长", ErrInvalid)
	}
	out.Game = model.Game{ID: "steam-" + id, Name: d.Name, Summary: summary, SourceURL: "https://store.steampowered.com/app/" + id + "/", Ports: []model.GamePort{}, Revision: 1}
	background := d.BackgroundRaw
	if background == "" {
		background = d.Background
	}
	for kind, raw := range map[string]string{"cover": d.Cover, "background": background} {
		b, err := fetchSteam(ctx, c, raw, 5<<20)
		if err != nil {
			return out, err
		}
		img, err := validateGameImage(b)
		if err != nil {
			return out, err
		}
		out.Images[kind] = img
	}
	return out, nil
}

func validateGameImage(b []byte) (gameImageData, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || (format != "jpeg" && format != "png") || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 16_000_000 || len(b) > 5<<20 {
		return gameImageData{}, fmt.Errorf("%w: 游戏图片须为不超过 5 MiB、1600 万像素的 JPEG/PNG", ErrInvalid)
	}
	if _, _, err = image.Decode(bytes.NewReader(b)); err != nil {
		return gameImageData{}, fmt.Errorf("%w: 游戏图片损坏", ErrInvalid)
	}
	return gameImageData{Data: b, ContentType: "image/" + format}, nil
}
