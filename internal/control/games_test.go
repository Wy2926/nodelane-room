package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/netip"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
)

func TestGamePortValidation(t *testing.T) {
	for _, ports := range [][]model.GamePort{
		{{Protocol: "tcp", Port: 0}}, {{Protocol: "icmp", Port: 80}},
		{{Protocol: "tcp", Port: 80, PortEnd: 79}},
		{{Protocol: "tcp", Port: 80, PortEnd: 82}, {Protocol: "tcp", Port: 82}},
	} {
		if err := model.ValidateGamePorts(ports); err == nil {
			t.Fatalf("accepted invalid ports: %+v", ports)
		}
	}
	must(t, model.ValidateGamePorts([]model.GamePort{{Protocol: "tcp", Port: 1, PortEnd: 65535}, {Protocol: "udp", Port: 1, PortEnd: 65535}}))

}

type steamTransport func(*http.Request) (*http.Response, error)

func (f steamTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func artwork(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	must(t, png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 4))))
	return b.Bytes()
}

func TestSteamImportValidationAndDownloads(t *testing.T) {
	for _, raw := range []string{"http://store.steampowered.com/app/105600/", "https://evil.example/app/105600/", "https://store.steampowered.com@127.0.0.1/app/105600/", "https://store.steampowered.com:443/app/105600/", "https://store.steampowered.com/bundle/105600/", "https://store.steampowered.com/app/0/", "https://store.steampowered.com/app/1%2F2/"} {
		if _, err := steamAppID(raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	id, err := steamAppID("https://store.steampowered.com/app/105600/Terraria/?l=schinese")
	must(t, err)
	if id != "105600" {
		t.Fatal(id)
	}
	for _, raw := range []string{"https://steamstatic.com.evil.test/a", "http://shared.steamstatic.com/a", "https://127.0.0.1/a", "https://shared.steamstatic.com:8080/a"} {
		if allowedSteamURL(raw) {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"127.0.0.1", "10.1.1.1", "::1", "::ffff:192.168.1.1", "169.254.169.254", "100.64.0.1", "198.18.0.1", "fe80::1"} {
		if publicGameIP(netip.MustParseAddr(raw)) {
			t.Fatal(raw)
		}
	}
	img := artwork(t)
	for _, tc := range []struct {
		name, metadata string
		badImage       bool
		wantError      bool
	}{
		{"valid", `{"105600":{"success":true,"data":{"type":"game","steam_appid":105600,"name":"Terraria","short_description":"<b>Build</b> &amp; play","header_image":"https://shared.steamstatic.com/cover.png","background_raw":"https://shared.steamstatic.com/background.png"}}}`, false, false},
		{"broken artwork", `{"105600":{"success":true,"data":{"type":"game","steam_appid":105600,"name":"Terraria","header_image":"https://shared.steamstatic.com/cover.png","background":"https://shared.steamstatic.com/background.png"}}}`, true, true},
		{"failed app", `{"105600":{"success":false}}`, false, true},
		{"mismatched app", `{"105600":{"success":true,"data":{"type":"game","steam_appid":1,"name":"Wrong"}}}`, false, true},
		{"SSRF image", `{"105600":{"success":true,"data":{"type":"game","steam_appid":105600,"name":"Bad","header_image":"https://localhost/a","background":"https://localhost/a"}}}`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := &http.Client{Transport: steamTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				data := img
				if r.URL.Host == "store.steampowered.com" {
					data = []byte(tc.metadata)
				} else if tc.badImage {
					data = []byte("<svg></svg>")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header), Request: r}, nil
			})}
			g, err := importSteamGame(context.Background(), id, c)
			if (err != nil) != tc.wantError {
				t.Fatalf("result %v", err)
			}
			if err == nil && (calls != 3 || g.Game.ID != "steam-105600" || g.Game.Enabled || len(g.Game.Ports) != 0 || g.Game.Summary != "Build & play" || len(g.Images) != 2 || !bytes.Equal(g.Images["cover"].Data, img)) {
				t.Fatalf("incomplete import: %+v calls=%d", g.Game, calls)
			}
		})
	}
	c := steamHTTPClient()
	defer c.CloseIdleConnections()
	req, _ := http.NewRequest("GET", "https://127.0.0.1/a", nil)
	if c.CheckRedirect(req, []*http.Request{req}) == nil {
		t.Fatal("private redirect allowed")
	}
	if _, err := validateGameImage(img[:len(img)/2]); err == nil {
		t.Fatal("truncated image accepted")
	}
}

func TestGameCatalogPolicyAcrossReplicas(t *testing.T) {
	s, a := newAdmin(t)
	ctx := context.Background()
	second := apiServer(t, &Store{Pool: s.Pool, Network: s.Network}, nil)
	host := user(t, second, "host")
	guest := user(t, second, "guest")
	var catalog []model.Game
	must(t, host.Call(ctx, "GET", "/v2/games", nil, &catalog))
	if len(catalog) != 2 {
		t.Fatal(catalog)
	}
	var room model.RoomResult
	must(t, host.Call(ctx, "POST", "/v2/rooms", model.RoomRequest{Name: "Configured", Game: "minecraft-java"}, &room))
	join(t, guest, room)
	path := "/v2/rooms/" + room.Room.ID
	var snap model.Snapshot
	must(t, host.Call(ctx, "GET", path, nil, &snap))
	if snap.Game.Network.Version != 1 || snap.Room.GameName != "Minecraft Java" {
		t.Fatal(snap)
	}
	in := model.GameUpdateRequest{Network: model.GameNetwork{Version: 1}, Name: "Server game", Ports: []model.GamePort{{Protocol: "udp", Port: 27015, PortEnd: 27016}}, Enabled: true, Revision: 1}
	status, b := a.request("PUT", "/games/minecraft-java", in, true, false)
	if status != 403 {
		t.Fatalf("CSRF: %d %s", status, b)
	}
	key := randomID()
	status, b = a.requestKey("PUT", "/games/minecraft-java", in, true, true, key)
	if status != 200 {
		t.Fatalf("update: %d %s", status, b)
	}
	must(t, host.Call(ctx, "GET", path, nil, &snap))
	if len(snap.Game.Ports) != 1 || snap.Game.Ports[0].Port != 27015 || snap.Game.Ports[0].PortEnd != 27016 {
		t.Fatal("obsolete game policy retained")
	}
	status, replayed := a.requestKey("PUT", "/games/minecraft-java", in, true, true, key)
	var firstGame, replayedGame model.Game
	must(t, json.Unmarshal(b, &firstGame))
	must(t, json.Unmarshal(replayed, &replayedGame))
	if status != 200 || !reflect.DeepEqual(firstGame, replayedGame) {
		t.Fatal("idempotent replay differs")
	}
	status, _ = a.request("PUT", "/games/minecraft-java", in, true, true)
	if status != 409 {
		t.Fatal("stale revision accepted")
	}
	in.Revision = 2
	in.Enabled = false
	status, b = a.request("PUT", "/games/minecraft-java", in, true, true)
	if status != 200 {
		t.Fatalf("disable: %d %s", status, b)
	}
	must(t, host.Call(ctx, "POST", path+"/heartbeat", model.HeartbeatRequest{LANVersion: 1}, nil))
	must(t, host.Call(ctx, "GET", path, nil, &snap))
	if snap.Game.Enabled {
		t.Fatal("disabled policy retained")
	}
	stranger := user(t, second, "stranger")
	statusError(t, stranger.Call(ctx, "POST", "/v2/rooms", model.RoomRequest{Name: "disabled", Game: "minecraft-java"}, nil), 403)
	statusError(t, stranger.Call(ctx, "POST", "/v2/rooms/join", model.JoinRequest{Code: room.Invitation.Code}, nil), 403)
	must(t, host.Call(ctx, "GET", "/v2/games", nil, &catalog))
	if len(catalog) != 1 || catalog[0].ID != "custom" {
		t.Fatal(catalog)
	}
	in.Revision = 1
	status, _ = a.request("PUT", "/games/custom", in, true, true)
	if status != 200 {
		t.Fatal("generic game must use the same administrator policy")
	}

}

func TestGameConfigConcurrentUpdatesAndOfflineExpiry(t *testing.T) {
	s, ca := database(t)
	server := apiServer(t, s, ca)
	a := user(t, server, "host")
	ctx := context.Background()
	var room model.RoomResult
	must(t, a.Call(ctx, "POST", "/v2/rooms", model.RoomRequest{Name: "Test", Game: "minecraft-java"}, &room))
	_, err := s.Pool.Exec(ctx, "UPDATE members SET last_seen=now()-interval '1 minute' WHERE room_id=$1", room.Room.ID)
	must(t, err)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for port := uint16(26001); port <= 26002; port++ {
		wg.Add(1)
		go func(port uint16) {
			defer wg.Done()
			results <- s.Write(ctx, func(tx pgx.Tx) error {
				_, err := s.updateGame(ctx, tx, "admin", "minecraft-java", model.GameUpdateRequest{Network: model.GameNetwork{Version: 1}, Name: "Test", Enabled: true, Revision: 1, Ports: []model.GamePort{{Protocol: "tcp", Port: port}}})
				return err
			})
		}(port)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
	var snap model.Snapshot
	must(t, a.Call(ctx, "GET", "/v2/rooms/"+room.Room.ID, nil, &snap))
	if time.Since(snap.Members[0].LastSeen) < 45*time.Second {
		t.Fatal("offline member authorization extended by game update")
	}
	must(t, a.Call(ctx, "POST", "/v2/rooms/"+room.Room.ID+"/heartbeat", model.HeartbeatRequest{LANVersion: 1}, nil))
	must(t, a.Call(ctx, "GET", "/v2/rooms/"+room.Room.ID, nil, &snap))
	if time.Since(snap.Members[0].LastSeen) > 5*time.Second || snap.Game.Revision != 2 {
		t.Fatal("heartbeat or concurrent configuration missing")
	}

}

func TestImportedArtworkAndOfflineIdempotentReplay(t *testing.T) {
	s, a := newAdmin(t)
	ctx := context.Background()
	img := artwork(t)
	key := randomID()
	response, err := s.mutateChecked(ctx, "admin:owner", key, hash("game-import:105600"), nil, func(tx pgx.Tx) (any, error) {
		if _, err := tx.Exec(ctx, "INSERT INTO games(id,name,source_url) VALUES('steam-105600','Terraria','https://store.steampowered.com/app/105600/')"); err != nil {
			return nil, err
		}
		for _, kind := range []string{"cover", "background"} {
			if _, err := tx.Exec(ctx, "INSERT INTO game_images(game_id,kind,data,content_type) VALUES('steam-105600',$1,$2,'image/png')", kind, img); err != nil {
				return nil, err
			}
		}
		return readGame(ctx, tx, "steam-105600")
	})
	must(t, err)
	status, b := a.requestKey("POST", "/games/import", model.GameImportRequest{URL: "https://store.steampowered.com/app/105600/"}, true, true, key)
	var storedGame, returnedGame model.Game
	must(t, json.Unmarshal(response, &storedGame))
	must(t, json.Unmarshal(b, &returnedGame))
	if status != 200 || !reflect.DeepEqual(storedGame, returnedGame) {
		t.Fatalf("replay: %d %s", status, b)
	}
	var g model.Game
	must(t, json.Unmarshal(b, &g))
	if g.CoverURL == "" || g.BackgroundURL == "" || g.Enabled {
		t.Fatal(g)
	}
	for _, path := range []string{g.CoverURL, g.BackgroundURL} {
		resp, err := a.server.Client().Get(a.server.URL + path)
		must(t, err)
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		must(t, err)
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" || !bytes.Equal(data, img) {
			t.Fatal("artwork not stored/served")
		}
	}
}

func TestSteamLiveImport(t *testing.T) {
	if os.Getenv("NODELANE_TEST_STEAM") != "1" {
		t.Skip("set NODELANE_TEST_STEAM=1 for an external Steam download smoke test")
	}
	g, err := importSteamGame(context.Background(), "105600", steamHTTPClient())
	must(t, err)
	if g.Game.Name == "" || len(g.Images) != 2 {
		t.Fatal("incomplete live import")
	}
	t.Logf("Steam app 105600: %s; cover=%d bytes, background=%d bytes", g.Game.Name, len(g.Images["cover"].Data), len(g.Images["background"].Data))
}

func TestLANPolicyCapabilitiesAndMACAcrossReplicas(t *testing.T) {
	s, admin := newAdmin(t)
	ctx := context.Background()
	server := apiServer(t, &Store{Pool: s.Pool, Network: s.Network}, nil)
	a, b := user(t, server, "alice"), user(t, server, "bob")
	in := model.GameUpdateRequest{Name: "LAN", Revision: 1, Enabled: true, Ports: []model.GamePort{{Protocol: "tcp", Port: 1, PortEnd: 65535}, {Protocol: "udp", Port: 1, PortEnd: 65535}}, Network: model.GameNetwork{Version: 1, Broadcast: true, Multicast: true, EthernetTypes: []uint16{0x8137}}}
	status, body := admin.request("PUT", "/games/minecraft-java", in, true, true)
	if status != 200 {
		t.Fatalf("LAN policy: %d %s", status, body)
	}
	var room model.RoomResult
	must(t, a.Call(ctx, "POST", "/v2/rooms", model.RoomRequest{Name: "LAN", Game: "minecraft-java"}, &room))
	join(t, b, room)
	path := "/v2/rooms/" + room.Room.ID
	statusError(t, a.Call(ctx, "POST", path+"/heartbeat", model.HeartbeatRequest{}, nil), 409)
	must(t, a.Call(ctx, "POST", path+"/heartbeat", model.HeartbeatRequest{LANVersion: 1, MAC: "02:00:00:00:00:01"}, nil))
	statusError(t, b.Call(ctx, "POST", path+"/heartbeat", model.HeartbeatRequest{LANVersion: 1, MAC: "02:00:00:00:00:01"}, nil), 409)
	statusError(t, b.Call(ctx, "POST", path+"/heartbeat", model.HeartbeatRequest{LANVersion: 1, MAC: "ff:ff:ff:ff:ff:ff"}, nil), 400)
	must(t, b.Call(ctx, "POST", path+"/heartbeat", model.HeartbeatRequest{LANVersion: 1, MAC: "02:00:00:00:00:02"}, nil))
	var snap model.Snapshot
	must(t, a.Call(ctx, "GET", path, nil, &snap))
	if snap.Game.Network.Version != 1 || len(snap.Game.Ports) != 2 {
		t.Fatal("LAN policy not shared or ranges expanded")
	}
	for _, m := range snap.Members {
		if m.MAC == "" {
			t.Fatal("MAC binding absent from snapshot")
		}
	}
	// Null/absent policy cannot reactivate the removed native L3 game path.
	in.Network = model.GameNetwork{}
	in.Revision = 2
	status, _ = admin.request("PUT", "/games/minecraft-java", in, true, true)
	if status != 400 {
		t.Fatal("missing LAN policy accepted")
	}
	for _, method := range []string{"POST", "DELETE"} {
		statusError(t, a.Call(ctx, method, path+"/endpoints", map[string]any{"protocol": "tcp", "port": 60000}, nil), 404)
	}
	var exists bool
	must(t, s.Pool.QueryRow(ctx, "SELECT to_regclass('endpoints') IS NOT NULL").Scan(&exists))
	if exists {
		t.Fatal("obsolete endpoint table retained")
	}

}
