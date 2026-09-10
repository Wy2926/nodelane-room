package control

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/model"
	"gopkg.in/yaml.v3"
)

func (s *Server) adminCreateNode(r *http.Request, tx pgx.Tx, actor string, raw []byte) (any, error) {
	var c model.NodeConfig
	if err := decodeBytes(raw, &c); err != nil {
		return nil, err
	}
	return s.Store.createNode(r.Context(), tx, actor, c)
}

func (s *Server) adminUpdateNode(r *http.Request, tx pgx.Tx, actor string, raw []byte) (any, error) {
	id := r.PathValue("node")
	var in struct {
		Config   model.NodeConfig `json:"config"`
		Revision int64            `json:"revision"`
	}
	if err := decodeBytes(raw, &in); err != nil {
		return nil, err
	}
	return s.Store.updateNode(r.Context(), tx, actor, id, in.Config, in.Revision)
}

func (s *Server) adminNodeAction(r *http.Request, tx pgx.Tx, actor string, raw []byte) (any, error) {
	id := r.PathValue("node")
	var in struct {
		Action string `json:"action"`
	}
	if err := decodeBytes(raw, &in); err != nil {
		return nil, err
	}
	return s.Store.nodeAction(r.Context(), tx, actor, id, in.Action)
}

func (s *Server) adminNodeKey(w http.ResponseWriter, r *http.Request, actor string) {
	ctx := r.Context()
	cookie, _ := r.Cookie(adminCookie)
	check := func(tx pgx.Tx) error { return validAdminSession(ctx, tx, hash(cookie.Value)) }
	if err := s.Store.Rate(ctx, "admin-key", 30, time.Minute); err != nil {
		s.fail(w, err)
		return
	}
	key, err := s.Store.issueEnrollmentKey(ctx, r.PathValue("node"), actor, check)
	s.result(w, map[string]any{"key": key, "expires_in": 1800}, err)
}

func (s *Server) adminNodeCompose(w http.ResponseWriter, r *http.Request, _ string) {
	tx, err := s.Store.Pool.BeginTx(r.Context(), pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	n, err := readNode(r.Context(), tx, r.PathValue("node"))
	if err != nil {
		s.fail(w, err)
		return
	}
	_, p, err := net.SplitHostPort(n.Address)
	if err != nil {
		s.fail(w, err)
		return
	}
	port, _ := strconv.Atoi(p)
	server := strings.TrimRight(s.PublicURL, "/")
	if server == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		server = scheme + "://" + r.Host
	}
	doc := map[string]any{"name": "nlroom-" + n.ID[:12], "services": map[string]any{"node": map[string]any{
		"image": "docker.nodelane.net/nodelane-room-node:" + model.Version, "command": []string{"run", "--server", server, "--listen-port", p}, "user": "0:0", "restart": "unless-stopped", "read_only": true, "cap_drop": []string{"ALL"}, "cap_add": []string{"NET_ADMIN"}, "security_opt": []string{"no-new-privileges:true"}, "devices": []string{"/dev/net/tun:/dev/net/tun"}, "ports": []string{p + ":" + p + "/udp"}, "volumes": []string{"/opt/nlroom-node/" + n.ID + "/state:/var/lib/nlroom-node"}, "tmpfs": []string{"/tmp:mode=1777,size=16m"}, "sysctls": map[string]string{"net.ipv4.ip_forward": "0"}, "stop_grace_period": "30s", "environment": map[string]string{"NLROOM_DEPLOYMENT": "container", "NLROOM_MAPPED_PORT": strconv.Itoa(port)}, "healthcheck": map[string]any{"test": []string{"CMD", "curl", "--fail", "--silent", "http://127.0.0.1:9090/healthz"}, "interval": "15s", "timeout": "3s"},
	}}}
	b, err := yaml.Marshal(doc)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Content-Disposition", `attachment; filename="compose.node.yaml"`)
	_, _ = w.Write(b)
}
