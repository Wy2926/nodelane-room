// Refresh V2 protocol schemas from Go types while retaining documented player endpoints.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
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
				if !strings.Contains(tag, "omitempty") {
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
	for _, v := range []any{model.Node{}, model.NodeConfig{}, model.EnrollmentChallengeRequest{}, model.NodeOperation{}, model.NodeReport{}, model.NodeProbe{}, model.NodeSyncRequest{}, model.NodeSync{}, model.Snapshot{}, model.Lease{}, model.LeaseRequest{}, control.AdminSnapshot{}, control.AdminRoomSnapshot{}} {
		schema(reflect.TypeOf(v))
	}
	schemes := components["securitySchemes"].(map[string]any)
	schemes["AdminCookie"] = M{"type": "apiKey", "in": "cookie", "name": "__Host-nlroom-admin", "description": "12 hour random session; 30 minute idle timeout. Secure, HttpOnly, SameSite=Strict. All admin writes also require exact Origin and X-CSRF-Token."}
	schemes["NodeBearer"] = M{"type": "http", "scheme": "bearer", "description": "One-hour session scoped to the current nonrevoked node identity binding. Cannot authorize player/admin APIs."}
	paths := doc["paths"].(map[string]any)
	paths["/v2/auth/verify"].(M)["post"].(M)["summary"] = "Verify Ed25519 signature over UTF8(nodelane-auth-v2:player:<id>:) followed by the raw nonce"
	doc["info"] = M{"title": "NodeLane Room V2", "version": model.Version, "description": "Fresh V2 database and identities required. No V1 API. Device Ed25519 proofs sign UTF-8 nodelane-auth-v2:<scope>:<challenge-id>: followed by raw challenge nonce; scope is player, node, or enrollment. Device identity and Nebula X25519 keys are separate. Byte fields use standard base64. Temporary enrollment keys contain 32 random bytes encoded as 64 hex characters, expire after 30 minutes and are consumed transactionally once. Never log credentials."}
	str := M{"type": "string"}
	empty := object(M{})
	ok := object(M{"ok": M{"type": "boolean", "enum": []bool{true}}}, "ok")
	password := M{"type": "string", "format": "password", "description": "12–128 UTF-8 bytes, not characters"}
	credentials := object(M{"username": str, "password": password}, "username", "password")
	bootstrap := object(M{"username": str, "password": password, "code": M{"type": "string", "minLength": 64, "maxLength": 64}}, "username", "password", "code")
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
	add("/v2/admin/bootstrap", "post", "Consume the 10 minute console bootstrap code and create the single administrator", "", bootstrap, ok, false)
	add("/v2/admin/login", "post", "Login; rate limited to 8/IP and 30 total per minute", "", credentials, session, false)
	add("/v2/admin/session", "get", "Restore current session and stable CSRF token", "AdminCookie", nil, session, false)
	add("/v2/admin/logout", "post", "Revoke current session", "AdminCookie", empty, ok, false)
	add("/v2/admin/password", "post", "Change password and revoke every admin session", "AdminCookie", object(M{"current": str, "password": password}, "current", "password"), ok, false)
	add("/v2/admin/snapshot", "get", "Current shared deployment, nodes, rooms, operations and recent audit snapshot", "AdminCookie", nil, ref("AdminSnapshot"), false)
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
	add("/v2/admin/rooms/{room}", "get", "Inspect room members and registered endpoints in one consistent snapshot", "AdminCookie", nil, ref("AdminRoomSnapshot"), false)
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
			if method == "get" || method == "post" || method == "put" {
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
