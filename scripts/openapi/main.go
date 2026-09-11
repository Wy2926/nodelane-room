// Generate current HTTP schemas and operation metadata.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/nodelane/nodelane-room/internal/control"
	"github.com/nodelane/nodelane-room/internal/model"
	"gopkg.in/yaml.v3"
)

type M = map[string]any

var schemas M
var generated map[string]bool

func ref(name string) M { return M{"$ref": "#/components/schemas/" + name} }
func schema(t reflect.Type) M {
	if t.Kind() == reflect.Pointer {
		return schema(t.Elem())
	}
	if t == reflect.TypeOf(time.Time{}) {
		return M{"type": "string", "format": "date-time"}
	}
	if t == reflect.TypeOf(json.RawMessage{}) {
		return M{"type": "object", "additionalProperties": true}
	}
	switch t.Kind() {
	case reflect.Bool:
		return M{"type": "boolean"}
	case reflect.String:
		return M{"type": "string"}
	case reflect.Int, reflect.Int64, reflect.Uint16, reflect.Uint64:
		return M{"type": "integer", "format": "int64"}
	case reflect.Float64:
		return M{"type": "number", "format": "double"}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return M{"type": "string", "format": "byte"}
		}
		return M{"type": "array", "items": schema(t.Elem())}
	case reflect.Map:
		return M{"type": "object", "additionalProperties": schema(t.Elem())}
	case reflect.Struct:
		name := t.Name()
		if generated[name] {
			return ref(name)
		}
		generated[name] = true
		schemas[name] = M{}
		properties := M{}
		required := []string{}
		var fields func(reflect.Type)
		fields = func(st reflect.Type) {
			for i := 0; i < st.NumField(); i++ {
				f := st.Field(i)
				if f.Anonymous {
					fields(f.Type)
					continue
				}
				tag := f.Tag.Get("json")
				parts := strings.Split(tag, ",")
				if parts[0] == "-" || parts[0] == "" {
					continue
				}
				properties[parts[0]] = schema(f.Type)
				if !strings.Contains(tag, "omitempty") && !strings.Contains(tag, "omitzero") {
					required = append(required, parts[0])
				}
			}
		}
		fields(t)
		schemas[name] = M{"type": "object", "properties": properties, "required": required}
		return ref(name)
	default:
		panic(t.String())
	}
}
func object(properties M, required ...string) M {
	return M{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	b, e := os.ReadFile("docs/openapi.yaml")
	if e != nil {
		return e
	}
	var doc M
	if e = yaml.Unmarshal(b, &doc); e != nil {
		return e
	}
	components := doc["components"].(map[string]any)
	schemas = components["schemas"].(map[string]any)

	generated = map[string]bool{}
	for _, v := range []any{model.Result{}, model.RoomResult{}, model.MemberRequest{}, model.AccountStatus{}, model.RoomPage{}, model.InviteInfo{}, model.TakeoverRequest{}, model.Operation{}} {
		schema(reflect.TypeOf(v))
	}
	schema(reflect.TypeOf(model.NetworkSample{}))
	schema(reflect.TypeOf(model.TelemetrySnapshot{}))
	for _, v := range []any{model.User{}, model.UserDetail{}, model.UserPage{}, model.UserAction{}, model.LoginStart{}, model.LoginAttempt{}, model.LoginClaim{}, model.LoginResult{}, model.OIDCSettings{}, model.Session{}, model.Game{}, model.GameUpdateRequest{}, model.GameImportRequest{}, model.RoomRequest{}, model.Node{}, model.NodeConfig{}, model.EnrollmentChallengeRequest{}, model.NodeOperation{}, model.NodeReport{}, model.NodeProbe{}, model.NodeSyncRequest{}, model.NodeSync{}, model.Snapshot{}, model.Lease{}, model.LeaseRequest{}, control.AdminSnapshot{}, control.AdminRoomSnapshot{}} {
		schema(reflect.TypeOf(v))
	}
	schema(reflect.TypeOf(model.RoomManagement{}))
	schema(reflect.TypeOf(model.HeartbeatRequest{}))
	schema(reflect.TypeOf(model.Status{}))
	for _, v := range []any{model.UpdateOverview{}, model.UpdateCheck{}, model.UpdateRepository{}, model.ClientReport{}} {
		schema(reflect.TypeOf(v))
	}
	schemes := components["securitySchemes"].(map[string]any)
	schemes["AdminCookie"] = M{"type": "apiKey", "in": "cookie", "name": "__Host-nlroom-admin", "description": "12 hour random session; 30 minute idle timeout. Secure, HttpOnly, SameSite=Strict. All admin writes also require exact Origin and X-CSRF-Token."}
	schemes["NodeBearer"] = M{"type": "http", "scheme": "bearer", "description": "One-hour session scoped to the current nonrevoked node identity binding. Cannot authorize player/admin APIs."}
	paths := doc["paths"].(map[string]any)

	paths["/v2/auth/verify"].(M)["post"].(M)["summary"] = "Verify Ed25519 signature over UTF8(nodelane-auth-v2:player:<id>:) followed by the raw nonce"
	doc["info"] = M{"title": "NodeLane Room V2", "version": model.ControlVersion, "description": "Fresh database schema version 6 and identities required; API remains /v2. No old data migration. Device Ed25519 proofs sign UTF-8 nodelane-auth-v2:<scope>:<challenge-id>: followed by raw challenge nonce; scope is guest, player, node, or enrollment. Device identity and Nebula X25519 keys are separate. Byte fields use standard base64. Temporary enrollment keys contain 32 random bytes encoded as 64 hex characters, expire after 30 minutes and are consumed transactionally once. Never log credentials."}
	str := M{"type": "string"}
	empty := object(M{})
	ok := object(M{"ok": M{"type": "boolean", "enum": []bool{true}}}, "ok")
	password := M{"type": "string", "format": "password", "description": "12–128 UTF-8 bytes, not characters"}
	credentials := object(M{"username": str, "password": password}, "username", "password")
	setupFields := M{"mode": M{"type": "string", "enum": []string{"create"}}, "username": str, "password": password, "code": M{"type": "string", "minLength": 64, "maxLength": 64, "writeOnly": true}, "database_url": M{"type": "string", "format": "password", "writeOnly": true}, "public_url": M{"type": "string", "description": "HTTPS origin or bare domain, must match the browser origin for creation"}, "network": M{"type": "string", "default": model.DefaultPool}, "registry": M{"type": "string", "default": control.DefaultRegistry}, "ca_mode": M{"type": "string", "enum": []string{"generate", "upload"}}, "ca_cert": str, "ca_key": M{"type": "string", "format": "password", "writeOnly": true}}
	setup := M{"oneOf": []any{object(setupFields, "mode", "code", "username", "password", "database_url", "public_url", "ca_mode"), object(M{"mode": M{"type": "string", "enum": []string{"connect"}}, "code": setupFields["code"], "username": str, "password": password, "database_url": setupFields["database_url"]}, "mode", "code", "username", "password", "database_url")}}
	session := object(M{"username": str, "csrf": str}, "username", "csrf")
	csrf := M{"name": "X-CSRF-Token", "in": "header", "required": true, "schema": M{"type": "string", "minLength": 64, "maxLength": 64}}
	origin := M{"name": "Origin", "in": "header", "required": true, "schema": str}
	add := func(path, method, summary, security string, in, out M, idempotent bool) {
		op := M{"summary": summary, "responses": M{"200": M{"description": "Success", "content": M{"application/json": M{"schema": out}}}, "400": M{"$ref": "#/components/responses/Error"}, "401": M{"$ref": "#/components/responses/Error"}, "403": M{"$ref": "#/components/responses/Error"}, "409": M{"$ref": "#/components/responses/Error"}, "429": M{"$ref": "#/components/responses/Error"}}}
		if security != "" {
			op["security"] = []any{M{security: []string{}}}
		} else {
			op["security"] = []any{}
		}
		params := []any{}
		for _, part := range strings.Split(path, "/") {
			if strings.HasPrefix(part, "{") {
				params = append(params, M{"name": strings.Trim(part, "{}"), "in": "path", "required": true, "schema": str})
			}
		}
		if method != "get" && strings.Contains(path, "/admin/") {
			params = append(params, origin)
			if security != "" {
				params = append(params, csrf)
			}
		}
		if idempotent {
			params = append(params, M{"name": "Idempotency-Key", "in": "header", "required": true, "schema": M{"type": "string", "minLength": 16, "maxLength": 128}})
		}
		if len(params) > 0 {
			op["parameters"] = params
		}
		if in != nil {
			op["requestBody"] = M{"required": true, "content": M{"application/json": M{"schema": in}}}
		}
		if paths[path] == nil {
			paths[path] = M{}
		}
		paths[path].(map[string]any)[method] = op
	}
	add("/v2/auth/guest/challenge", "post", "Begin explicit guest registration; 100 requests per source IP per hour", "", ref("ChallengeRequest"), ref("Challenge"), false)
	add("/v2/auth/guest/verify", "post", "Prove the device key and atomically create a guest user; existing device proofs cannot change ownership or restore revoked grants", "", ref("VerifyRequest"), ref("Session"), false)
	add("/v2/auth/challenge", "post", "Challenge an existing authorized account device; never creates a player account", "", ref("ChallengeRequest"), ref("Challenge"), false)
	add("/v2/auth/verify", "post", "Verify player-scoped device proof against active user and nonrevoked device grant", "", ref("VerifyRequest"), ref("Session"), false)
	add("/v2/me", "get", "Current account, distinct from the device and paid entitlements", "Bearer", nil, ref("User"), false)
	add("/v2/client/report", "post", "Authenticated device service/GUI version and update result; independent of room membership", "Bearer", ref("ClientReport"), ok, true)
	add("/v2/updates/check", "get", "Public update policy, signed TUF metadata and ordered HTTPS package URLs; no-store, 120/IP/minute", "", nil, ref("UpdateCheck"), false)
	paths["/v2/updates/check"].(M)["get"].(M)["parameters"] = []any{M{"name": "version", "in": "query", "required": true, "schema": str}, M{"name": "os", "in": "query", "required": true, "schema": M{"type": "string", "enum": []string{"windows", "linux"}}}, M{"name": "arch", "in": "query", "required": true, "schema": M{"type": "string", "enum": []string{"amd64", "arm64"}}}, M{"name": "installed", "in": "query", "schema": M{"type": "string", "enum": []string{"1"}}, "description": "Retrieve the exact installed release for a verified Linux rollback package; includes published and paused releases."}}
	add("/v2/admin/updates", "get", "Update sources, latest 200 releases, policies, version distribution, latest 100 device/release results and 100 devices", "AdminCookie", nil, ref("UpdateOverview"), false)
	paths["/v2/admin/updates"].(M)["get"].(M)["parameters"] = []any{M{"name": "after", "in": "query", "schema": str}}
	for _, v := range []struct{ path, model, summary string }{{"sources", "UpdateSource", "Save R2/S3/HTTPS configuration with original revision; secrets are encrypted and write-only"}, {"repository", "UpdateRepository", "Import at most 3 MiB signed TUF metadata with original revision; signature, expiry and monotonic trusted versions required"}, {"releases", "UpdateRelease", "Create immutable signed target draft or change notes/state with original revision; publication requires a verified enabled source"}, {"policies", "UpdatePolicy", "Set recommendation and optional minimum version/deadline per platform with original revision; empty release_id clears policy and preserves revision"}} {
		add("/v2/admin/updates/"+v.path, "put", v.summary, "AdminCookie", ref(v.model), ref(v.model), true)
	}
	add("/v2/admin/updates/sources/{source}/test", "post", "Test stored source connectivity; does not verify a package or modify publication", "AdminCookie", empty, ok, false)
	add("/v2/admin/updates/releases/{release}/sources/{source}", "post", "Verify the complete signed size and SHA256 of an existing object before recording a replica", "AdminCookie", empty, ok, false)
	add("/v2/admin/updates/releases/{release}/sources/{source}", "put", "Upload immutable full package to R2/S3 and verify download; maximum signed size 2 GiB; repeat verifies existing object", "AdminCookie", nil, ok, false)
	paths["/v2/admin/updates/releases/{release}/sources/{source}"].(M)["put"].(M)["requestBody"] = M{"required": true, "content": M{"application/octet-stream": M{"schema": M{"type": "string", "format": "binary"}}}}
	for _, name := range []string{"access_key", "secret_key"} {
		schemas["UpdateSource"].(M)["properties"].(M)[name].(M)["writeOnly"] = true
	}
	paths["/v2/admin/updates/repository"].(M)["put"].(M)["responses"].(M)["200"].(M)["content"].(M)["application/json"].(M)["schema"] = object(M{"revision": M{"type": "integer", "format": "int64"}}, "revision")
	schemas["UpdatePolicy"].(M)["description"] = "At the deadline, below-minimum or unreported devices lose existing network grants and create/join/lease/heartbeat return update_required before idempotency replay. Update check, report, diagnostics and account management remain available. Versions are authenticated self-reports, not remote attestation."
	add("/v2/me/logout", "post", "Revoke this registered account device and its live network authorization; guest must link first", "Bearer", empty, ok, true)
	add("/v2/me/takeover", "post", "Explicitly release this account's other devices from rooms; current device can then join", "Bearer", empty, ok, true)
	add("/v2/me/identity", "post", "Link one OIDC identity to the authenticated guest without changing user ID; identity conflict never merges accounts", "Bearer", ref("LoginStart"), ref("LoginAttempt"), false)
	add("/v2/auth/oidc/start", "post", "Begin browser login for a new device key; 5-minute transaction", "", ref("LoginStart"), ref("LoginAttempt"), false)
	add("/v2/auth/oidc/claim", "post", "Poll and consume login using the original device signature and private proof", "", ref("LoginClaim"), ref("LoginResult"), false)
	add("/v2/admin/oidc", "get", "Read the configured issuer, client ID and enabled state; never returns the secret", "AdminCookie", nil, ref("OIDCSettings"), false)
	add("/v2/admin/oidc", "put", "Configure one OIDC provider and invalidate pending logins; omitted secret retains the credential only for the same issuer and client ID", "AdminCookie", ref("OIDCSettings"), ok, true)
	add("/v2/admin/users", "get", "List 50 users ordered by ID, optional q nickname/ID and after cursor", "AdminCookie", nil, ref("UserPage"), false)
	paths["/v2/admin/users"].(M)["get"].(M)["parameters"] = []any{M{"name": "q", "in": "query", "schema": str}, M{"name": "after", "in": "query", "schema": str}}
	add("/v2/admin/users/{user}", "get", "Consistent account, devices, up to 100 rooms and up to 100 active session metadata; no credentials", "AdminCookie", nil, ref("UserDetail"), false)
	add("/v2/admin/users/{user}/actions", "post", "Enable/disable room creation while retaining sessions and existing rooms; delete closes owned rooms and revokes network authority; logout/revoke-device ends device authorization; reason required", "AdminCookie", ref("UserAction"), ok, true)
	for _, route := range []struct{ path, method string }{{"browser", "get"}, {"callback", "get"}, {"confirm", "post"}} {
		path := "/v2/auth/oidc/" + route.path
		add(path, route.method, "OIDC browser flow; browser-bound state, PKCE S256, nonce, and explicit device confirmation", "", nil, str, false)
		paths[path].(M)[route.method].(M)["responses"].(M)["200"] = M{"description": "Escaped HTML confirmation or completion page", "content": M{"text/html": M{"schema": str}}}
		paths[path].(M)[route.method].(M)["responses"].(M)["303"] = M{"description": "Redirect to configured OIDC authorization endpoint"}
	}
	paths["/v2/auth/oidc/browser"].(M)["get"].(M)["parameters"] = []any{M{"name": "id", "in": "query", "required": true, "schema": str}}
	paths["/v2/auth/oidc/callback"].(M)["get"].(M)["parameters"] = []any{M{"name": "state", "in": "query", "required": true, "schema": str}, M{"name": "code", "in": "query", "schema": str}, M{"name": "error", "in": "query", "schema": str}}
	paths["/v2/auth/oidc/confirm"].(M)["post"].(M)["requestBody"] = M{"required": true, "content": M{"application/x-www-form-urlencoded": M{"schema": object(M{"id": str, "csrf": str}, "id", "csrf")}}}
	paths["/v2/auth/oidc/callback"].(M)["get"].(M)["description"] = "Confirmation HTML uses Referrer-Policy: strict-origin to preserve the form POST Origin without exposing callback code or state in Referer."
	paths["/v2/auth/oidc/confirm"].(M)["post"].(M)["description"] = "Requires the control service Origin, the login transaction browser cookie and its CSRF token. Missing, null or foreign Origin and cross-site requests are rejected."
	schemas["LoginStart"].(M)["description"] = "proof is SHA256 of a 64-hex-character random private proof. Sign UTF8(nodelane-login-start:<device_id>:<proof_hash>) with the device Ed25519 key. Only a valid device signature can start enrollment."
	schemas["LoginClaim"].(M)["description"] = "proof is the original private proof; sign UTF8(nodelane-login-claim:<id>:<proof>). A successful claim is consumed once. Never log proof or returned sessions."
	schemas["OIDCSettings"].(M)["properties"].(M)["client_secret"].(M)["writeOnly"] = true
	schemas["Room"].(M)["properties"].(M)["capacity"].(M)["default"] = model.RoomCapacity

	delete(paths, "/v2/admin/bootstrap")
	add("/v2/rooms", "get", "This player's owned, open, unexpired rooms; newest expiry first, at most 500", "Bearer", nil, M{"type": "array", "items": ref("Room")}, false)
	add("/v2/rooms/{room}/manage", "get", "Owner-only consistent room, game, member and game-policy view, including after leaving; never grants tunnel authority", "Bearer", nil, ref("RoomManagement"), false)
	doc["x-local-api"] = M{
		"transport":        "HTTP over owner-authorized Named Pipe (Windows) or Unix socket (Linux); not served on the public control listener",
		"protocol_version": 3,
		"rpc": M{"method": "POST", "path": "/rpc", "max_request_bytes": 65536, "max_response_bytes": 4 << 20,
			"request":   object(M{"action": M{"type": "string", "enum": []string{"init", "account-login", "account-link", "account-poll", "account-cancel", "account-logout", "account-takeover", "update-status", "update-check", "update-install", "status", "games", "rooms", "manage", "members", "create", "join", "invite", "kick", "transfer", "leave", "close", "ping", "doctor"}}, "room": str, "server": str, "name": str, "target": str, "body": M{"type": "object"}}, "action"),
			"error":     object(M{"code": str, "error": str}, "code", "error"),
			"responses": M{"update-status": ref("UpdateStatus"), "update-check": ref("UpdateStatus"), "update-install": object(M{"elevate": M{"type": "boolean"}}, "elevate"), "status": ref("Status"), "games": M{"type": "array", "items": ref("Game")}, "rooms": ref("RoomPage"), "manage": ref("RoomManagement"), "members": ref("Snapshot")}},
		"images": M{"method": "GET", "path": "/game-images/{game}/{kind}", "kind": []string{"cover", "background"}, "max_response_bytes": 5 << 20, "description": "Public JPEG/PNG fetched only from the configured control origin, without session or redirects; at most two concurrent downloads; never part of status."},
	}
	add("/v2/games", "get", "Enabled server game catalog; all games use the mandatory Ethernet LAN policy", "Bearer", nil, M{"type": "array", "items": ref("Game")}, false)
	add("/v2/games/{game}/images/{image}", "get", "Downloaded public game artwork; image is cover or background", "", nil, str, false)
	paths["/v2/games/{game}/images/{image}"].(M)["get"].(M)["responses"] = M{"200": M{"description": "Validated JPEG/PNG from shared PostgreSQL, at most 5 MiB", "content": M{"image/jpeg": M{"schema": M{"type": "string", "format": "binary"}}, "image/png": M{"schema": M{"type": "string", "format": "binary"}}}}, "404": M{"$ref": "#/components/responses/Error"}}
	add("/v2/admin/games/import", "post", "Import a Steam app link, download both images and atomically save a disabled draft", "AdminCookie", ref("GameImportRequest"), ref("Game"), true)
	paths["/v2/admin/games/import"].(M)["post"].(M)["description"] = "Only https://store.steampowered.com/app/<id>/ links; fixed store API and HTTPS steamstatic.com artwork, public resolved addresses, bounded redirects/downloads. No API key. 10 imports/admin/minute, at most 500 games. External failure stores nothing; a completed retry returns its cached response. Steam Store appdetails availability and fields may change. Ports are configured manually after import."
	add("/v2/admin/games/{game}", "put", "Update name, TCP/UDP port intervals, Ethernet LAN policy and enabled state using the original revision", "AdminCookie", ref("GameUpdateRequest"), ref("Game"), true)
	add("/v2/rooms/{room}/heartbeat", "post", "Renew membership; LAN rooms require lan_version=1 and register the local unicast MAC", "Bearer", ref("HeartbeatRequest"), ok, true)
	paths["/v2/admin/games/{game}"].(M)["put"].(M)["description"] = "TCP/UDP port intervals cover 1–65535 with no port-count limit; overlapping intervals are rejected. network.version=1 is required for all games, including custom; Ethernet LAN with broadcast/multicast and explicit extra non-IP EtherTypes (0 for IEEE 802.3/LLC). Enabled games require ports or non-IP rules. Policy, revision and room/admin events update transactionally in database schema 6; no schema migration. Disabling withdraws game authorization."
	schemas["RoomRequest"].(M)["properties"].(M)["game"].(M)["description"] = "Enabled game ID from GET /v2/games, or custom; no game-specific discovery."
	add("/v2/admin/setup", "get", "Check local database configuration and loaded control plane readiness", "", nil, object(M{"initialized": M{"type": "boolean"}, "configured": M{"type": "boolean"}}, "initialized", "configured"), false)
	add("/v2/admin/setup", "post", "Use a 10 minute local console code to create a control plane or connect an independent instance", "", setup, object(M{"ok": M{"type": "boolean"}, "public_url": str}, "ok", "public_url"), false)
	paths["/v2/admin/setup"].(M)["post"].(M)["description"] = "Available before the database is configured. Requires the same browser Origin and the current instance console code. create atomically initializes an empty or unused current schema (version 6), configuration, CA and administrator; existing deployments and older schemas are rejected without modification. upload requires exactly one matching CA certificate and private key; generate rejects supplied CA material. connect authenticates an existing administrator, rate limited to 8/minute across the shared database, and loads stored configuration without changing it. Only the database locator is persisted privately on each instance for restart; all shared configuration and CA are in PostgreSQL. Maximum JSON body 65536 bytes. Success precedes asynchronous instance readiness; poll GET setup or /readyz."
	add("/v2/admin/login", "post", "Login; rate limited to 8/IP and 30 total per minute", "", credentials, session, false)
	add("/v2/admin/session", "get", "Restore current session and stable CSRF token", "AdminCookie", nil, session, false)
	add("/v2/admin/logout", "post", "Revoke current session", "AdminCookie", empty, ok, false)
	add("/v2/admin/password", "post", "Change password and revoke every admin session", "AdminCookie", object(M{"current": str, "password": password}, "current", "password"), ok, false)
	add("/v2/admin/snapshot", "get", "Current shared deployment, nodes, rooms, operations and recent audit snapshot", "AdminCookie", nil, ref("AdminSnapshot"), false)
	add("/v2/admin/telemetry", "get", "Current instance memory only: 60-second observations, stale after 15 seconds; no durable cursor", "AdminCookie", nil, ref("TelemetrySnapshot"), false)
	paths["/v2/admin/telemetry"].(M)["get"].(M)["description"] = "GeoIP is true only while a local MMDB is loaded. DB-IP City Lite is downloaded and refreshed automatically by default; lookups never send peer IPs to an external service. Failed updates retain the last valid cache. The browser entry is instance-local, randomly generated or configured with NODELANE_ADMIN_PATH, and available only through the local admin path command. GET / and /admin return 404 without redirects; API paths and authentication are unchanged."
	add("/v2/node/telemetry", "post", "Authenticated node observations; 128 peers per report, 5-second cadence, no persistence", "NodeBearer", ref("NetworkSample"), ok, false)
	add("/v2/rooms/{room}/telemetry", "post", "Active member observations scoped to this room; no persistence or idempotency record", "Bearer", ref("NetworkSample"), ok, false)
	add("/v2/admin/events", "get", "SSE full authoritative snapshots with durable event IDs; reconnect every five minutes", "AdminCookie", nil, ref("AdminSnapshot"), false)
	ev := paths["/v2/admin/events"].(M)["get"].(M)
	ev["description"] = "Each event is named snapshot, with AdminSnapshot JSON data and its durable revision as id. Last-Event-ID is accepted; old, pruned or future cursors recover via the complete current snapshot. Does not replay secrets. Session validity checked on each event."
	ev["parameters"] = []any{M{"name": "Last-Event-ID", "in": "header", "schema": str}}
	ev["responses"].(M)["200"] = M{"description": "SSE snapshot stream", "content": M{"text/event-stream": M{"schema": str}}}
	add("/v2/admin/nodes", "post", "Pre-create an authorized logical node", "AdminCookie", ref("NodeConfig"), ref("Node"), true)
	add("/v2/admin/nodes/{node}", "put", "Optimistic desired configuration update; invalidates pending keys", "AdminCookie", object(M{"config": ref("NodeConfig"), "revision": M{"type": "integer"}}, "config", "revision"), ref("Node"), true)
	add("/v2/admin/nodes/{node}/key", "post", "Issue one-use temporary key; replaces prior key, plaintext shown only once and never cached", "AdminCookie", empty, object(M{"key": str, "expires_in": M{"type": "integer", "enum": []int{1800}}}, "key", "expires_in"), false)
	add("/v2/admin/nodes/{node}/actions", "post", "Create generation-bound node lifecycle operation", "AdminCookie", object(M{"action": M{"type": "string", "enum": []string{"drain", "disable", "resume", "restart", "replace", "revoke", "revoke-key"}}}, "action"), ref("NodeOperation"), true)
	add("/v2/admin/nodes/{node}/compose", "get", "Download versioned node Compose without enrollment secrets", "AdminCookie", nil, str, false)
	paths["/v2/admin/nodes/{node}/compose"].(M)["get"].(M)["responses"].(M)["200"] = M{"description": "Compose YAML attachment", "content": M{"application/yaml": M{"schema": str}}}
	add("/v2/admin/rooms/{room}", "get", "Inspect room members and game policy in one consistent snapshot", "AdminCookie", nil, ref("AdminRoomSnapshot"), false)
	add("/v2/admin/rooms/{room}/actions", "post", "Close room or kick a member and revoke authorization", "AdminCookie", object(M{"action": M{"type": "string", "enum": []string{"close", "kick"}}, "device_id": str}, "action"), ref("Room"), true)
	add("/v2/node/enrollment/challenge", "post", "Validate temporary key before creating a challenge; no public requests", "", ref("EnrollmentChallengeRequest"), ref("Challenge"), false)
	add("/v2/node/enrollment/complete", "post", "Prove persisted identity and atomically consume key and bind address", "", ref("VerifyRequest"), ref("Session"), false)
	add("/v2/node/auth/challenge", "post", "Authenticate an existing binding, including interrupted enrollment recovery", "", ref("ChallengeRequest"), ref("Challenge"), false)
	add("/v2/node/auth/verify", "post", "Verify node-scoped Ed25519 proof", "", ref("VerifyRequest"), ref("Session"), false)
	add("/v2/node/sync", "post", "5 second heartbeat, desired configuration and idempotent operation feedback", "NodeBearer", ref("NodeSyncRequest"), ref("NodeSync"), false)
	add("/v2/node/lease", "post", "Issue/renew <=10 minute Nebula certificate; disabled/revoked bindings denied", "NodeBearer", ref("LeaseRequest"), ref("Lease"), true)
	add("/install/{file}", "get", "Read-only node.sh, manifest.json, SHA256SUMS or exact-version native archive", "", nil, str, false)
	paths["/install/{file}"].(M)["get"].(M)["responses"] = M{
		"200": M{"description": "Raw file bytes; Content-Type depends on the selected file. manifest.json is a JSON object, scripts/checksums are text, and archives are gzip data.", "content": M{
			"application/json": M{"schema": M{"type": "object", "additionalProperties": true}},
			"text/*":           M{"schema": str},
			"*/*":              M{"schema": M{"type": "string", "format": "binary"}},
		}},
		"404": M{"description": "Unknown or unavailable release file"},
	}
	for _, t := range []string{"NodeConfig", "NodeSyncRequest", "EnrollmentChallengeRequest"} {
		schemas[t].(M)["additionalProperties"] = false
	}
	add("/v2/capabilities", "get", "Public interaction contract, LAN, login availability and business readiness", "", nil, object(M{"contract": str, "lan_version": M{"type": "integer"}, "local_protocol_version": M{"type": "integer"}, "oidc_enabled": M{"type": "boolean"}, "ready": M{"type": "boolean"}}, "contract", "lan_version", "local_protocol_version", "oidc_enabled", "ready"), false)
	add("/v2/me", "get", "Current device grant, room creation permission with reason, own membership and same-account occupancy; no credentials", "Bearer", nil, ref("AccountStatus"), false)
	add("/v2/me/takeover", "post", "Release the explicitly confirmed other device occupancy; never joins another room", "Bearer", ref("TakeoverRequest"), ok, true)
	add("/v2/me/operations/{operation}", "get", "Read this authorized device's receipt; missing does not establish nonexecution", "Bearer", nil, ref("Operation"), false)
	add("/v2/me/devices", "get", "List this registered player's own device grants", "Bearer", nil, M{"type": "array", "items": ref("UserDevice")}, false)
	add("/v2/me/devices/{device}/revoke", "post", "Revoke another device of this account; use logout for the current device", "Bearer", empty, ok, true)
	add("/v2/rooms", "get", "Own open unexpired management rooms, bounded to 500 with explicit truncation", "Bearer", nil, ref("RoomPage"), false)
	add("/v2/rooms/{room}/invite", "get", "Current invitation metadata only; no code or hash", "Bearer", nil, ref("InviteInfo"), false)
	add("/v2/rooms/{room}/invite/revoke", "post", "Invalidate the invitation with the confirmed room revision", "Bearer", ref("MemberRequest"), ref("Room"), true)
	add("/v2/rooms/{room}/join", "post", "Owner joins by room ID subject to the same capacity, ban, game and account checks", "Bearer", ref("MemberRequest"), ref("RoomResult"), true)
	add("/v2/rooms/{room}/heartbeat", "post", "Accept LAN heartbeat; replay never extends membership authorization", "Bearer", ref("HeartbeatRequest"), object(M{"accepted_at": M{"type": "string", "format": "date-time"}, "membership_valid_until": M{"type": "string", "format": "date-time"}}, "accepted_at", "membership_valid_until"), true)
	add("/v2/admin/operations/{operation}", "get", "Read the current administrator's mutation receipt", "AdminCookie", nil, ref("Operation"), false)
	add("/v2/admin/nodes/{node}/operations/{operation}", "get", "Exact node execution status; independent from acceptance of an admin mutation", "AdminCookie", nil, ref("NodeOperation"), false)
	for _, path := range []string{"/v2/admin/nodes/{node}/actions", "/v2/admin/rooms/{room}/actions"} {
		body := paths[path].(M)["post"].(M)["requestBody"].(M)["content"].(M)["application/json"].(M)["schema"].(M)
		body["properties"].(M)["expected_revision"] = M{"type": "integer", "minimum": 1}
		body["required"] = append(body["required"].([]string), "expected_revision")
	}
	resultProperties := schemas["Result"].(M)["properties"].(M)
	resultProperties["data"] = M{"nullable": true, "description": "Operation-specific payload, including arrays or scalars; null for errors."}
	resultProperties["contract"] = M{"type": "string", "enum": []string{model.Contract}}
	resultProperties["origin"] = M{"type": "string", "enum": []string{"control", "service", "bridge"}}
	resultProperties["control_http_status"] = M{"type": "integer", "nullable": true, "description": "Actual upstream HTTP status when observed; null without a control response."}
	codes := []string{}
	catalog := M{}
	for code, spec := range model.BusinessCodes {
		codes = append(codes, code)
		catalog[code] = M{"http_status": spec.Status, "default_message": spec.Message, "retry": spec.Retry}
	}
	sort.Strings(codes)
	resultProperties["code"] = M{"type": "string", "enum": codes}
	doc["x-business-codes"] = catalog
	schemas["Retry"].(M)["properties"].(M)["kind"] = M{"type": "string", "enum": []string{"none", "manual", "after", "reauth", "refresh"}}
	schemas["Details"].(M)["additionalProperties"] = false
	schemas["Details"].(M)["description"] = "Per-code allowlist: validation fields/rules; stale expected/actual revision; occupancy room/device/revision; room_full capacity/member_count; request_too_large allowed_bytes; public expiry expires_at. known_commit only when a prior write was confirmed. Never request data, credentials or raw exceptions."
	schemas["Error"] = ref("Result")
	components["responses"].(M)["Error"] = M{"description": "Structured interaction-1 result; code determines recovery, never message or HTTP class alone.", "content": M{"application/json": M{"schema": ref("Result")}}}
	envelope := func(data any) M {
		return M{"x-interaction-envelope": true, "allOf": []any{ref("Result"), M{"type": "object", "properties": M{"data": data}}}}
	}
	for path, value := range paths {
		if !strings.HasPrefix(path, "/v2/") {
			continue
		}
		for method, raw := range value.(M) {
			if method != "get" && method != "post" && method != "put" && method != "delete" {
				continue
			}
			op := raw.(M)
			responses := op["responses"].(M)
			for _, status := range []string{"200", "201", "202"} {
				response, ok := responses[status].(M)
				if !ok {
					continue
				}
				content, ok := response["content"].(M)
				if !ok {
					continue
				}
				jsonContent, ok := content["application/json"].(M)
				if !ok {
					continue
				}
				data := jsonContent["schema"]
				if previous, ok := data.(M); ok && previous["x-interaction-envelope"] == true {
					data = previous["allOf"].([]any)[1].(M)["properties"].(M)["data"]
				}
				jsonContent["schema"] = envelope(data)
			}
			for _, status := range []string{"400", "401", "403", "404", "405", "409", "410", "413", "415", "422", "429", "500", "503", "504"} {
				responses[status] = M{"$ref": "#/components/responses/Error"}
			}
			params, _ := op["parameters"].([]any)
			filtered := []any{}
			idempotent := false
			for _, param := range params {
				p := param.(M)
				if p["name"] == model.ContractHeader || p["name"] == model.DeadlineHeader {
					continue
				}
				if p["name"] == "Idempotency-Key" {
					idempotent = true
				}
				filtered = append(filtered, p)
			}
			public := path == "/v2/capabilities" || path == "/v2/updates/check" || strings.Contains(path, "/images/") || strings.HasSuffix(path, "/compose") || path == "/v2/auth/oidc/browser" || path == "/v2/auth/oidc/callback" || path == "/v2/auth/oidc/confirm"
			if !public {
				filtered = append(filtered, M{"name": model.ContractHeader, "in": "header", "required": true, "schema": M{"type": "string", "enum": []string{model.Contract}}})
			}
			if idempotent {
				filtered = append(filtered, M{"name": model.DeadlineHeader, "in": "header", "required": true, "schema": M{"type": "string", "format": "date-time"}, "description": "Immutable deadline, future and at most database time + 1 hour on first acceptance. Binds raw body, method and path to Idempotency-Key. Expired requests cannot execute."})
			}
			op["parameters"] = filtered
			if method == "post" && (path == "/v2/rooms" || path == "/v2/admin/nodes") {
				if responses["200"] != nil {
					responses["201"] = responses["200"]
					delete(responses, "200")
				}
			}
		}
	}
	paths["/v2/admin/nodes/{node}/actions"].(M)["post"].(M)["responses"].(M)["202"] = M{"description": "Command accepted; data identifies the independently tracked node operation.", "content": M{"application/json": M{"schema": envelope(ref("NodeOperation"))}}}
	localRPC := doc["x-local-api"].(M)["rpc"].(M)
	localRequest := localRPC["request"].(M)
	localFields := localRequest["properties"].(M)
	localFields["contract"] = M{"type": "string", "enum": []string{model.Contract}}
	localFields["command_id"] = M{"type": "string", "minLength": 16, "maxLength": 128}
	localRequest["required"] = []string{"contract", "action"}
	actions := localFields["action"].(M)["enum"].([]string)
	actions = append(actions, "capabilities", "get-operation", "network-retry", "network-stop", "invite-info", "invite-revoke", "owner-join", "account-devices", "account-status", "revoke-device")
	localFields["action"].(M)["enum"] = actions
	localRPC["error"] = ref("Result")
	localRPC["response"] = ref("Result")
	localRPC["description"] = "Protocol 3. Mutations require stable command_id; the protected service ledger freezes the raw request and deadline before submission. Recover through get-operation and status.pending_operations. No legacy protocol support. Peer rtt_ms and loss_percent summarize completed background probes from the last 30 seconds; measured_at is the latest completed probe time. RTT averages successful replies only; all timeouts yield 100 percent loss without RTT, and no recent samples omit both values."
	localRPC["responses"].(M)["get-operation"] = ref("Operation")
	for _, path := range []string{"/v2/rooms/{room}/events", "/v2/admin/events"} {
		paths[path].(M)["get"].(M)["x-events"] = M{"snapshot": "Current authoritative snapshot; former room members receive only self tombstones", "reset": ref("Result"), "terminal": ref("Result")}
	}
	doc["tags"] = []any{M{"name": "Player"}, M{"name": "Admin"}, M{"name": "Node"}, M{"name": "Operations"}}
	for path, value := range paths {
		tag := "Operations"
		switch {
		case strings.HasPrefix(path, "/v2/admin/"):
			tag = "Admin"
		case strings.HasPrefix(path, "/v2/node/"):
			tag = "Node"
		case strings.HasPrefix(path, "/v2/"):
			tag = "Player"
		}
		for method, operation := range value.(M) {
			if method == "get" || method == "post" || method == "put" || method == "delete" {
				operation.(M)["tags"] = []string{tag}
			}
		}
	}
	b, e = yaml.Marshal(doc)
	if e != nil {
		return e
	}
	return os.WriteFile("docs/openapi.yaml", b, 0644)
}
