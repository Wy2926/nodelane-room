package control

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
)

const DefaultRegistry = "docker.nodelane.net"

type SetupRequest struct {
	Mode        string `json:"mode"`
	Code        string `json:"code"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	DatabaseURL string `json:"database_url"`
	Network     string `json:"network"`
	PublicURL   string `json:"public_url"`
	Registry    string `json:"registry"`
	CAMode      string `json:"ca_mode"`
	CACert      string `json:"ca_cert"`
	CAKey       string `json:"ca_key"`
}

func (in *SetupRequest) normalize() error {
	if !model.ValidLabel(in.Username, 80) || len(in.Password) < 12 || len(in.Password) > 128 {
		return fmt.Errorf("%w: 管理员账号无效，密码须为 12–128 字节", ErrInvalid)
	}
	in.DatabaseURL = strings.TrimSpace(in.DatabaseURL)
	u, err := url.Parse(in.DatabaseURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.Path == "" {
		return fmt.Errorf("%w: 请填写完整 PostgreSQL 连接串", ErrInvalid)
	}
	if in.Mode == "connect" {
		if in.PublicURL != "" || in.Network != "" || in.Registry != "" || in.CAMode != "" || in.CACert != "" || in.CAKey != "" {
			return ErrInvalid
		}
		return nil
	}
	if in.Mode != "create" {
		return ErrInvalid
	}
	in.PublicURL = strings.TrimSpace(in.PublicURL)
	if !strings.Contains(in.PublicURL, "://") {
		in.PublicURL = "https://" + in.PublicURL
	}
	in.PublicURL = strings.TrimRight(in.PublicURL, "/")
	if device.ValidateURL(in.PublicURL) != nil {
		return fmt.Errorf("%w: 公网地址须为 HTTPS 域名或 origin，不含路径", ErrInvalid)
	}
	u, err = url.Parse(in.PublicURL)
	if err != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return ErrInvalid
	}
	in.Network = strings.TrimSpace(in.Network)
	if in.Network == "" {
		in.Network = model.DefaultPool
	}
	in.Registry = strings.TrimSpace(in.Registry)
	if in.Registry == "" {
		in.Registry = DefaultRegistry
	}
	u, err = url.Parse("https://" + in.Registry)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(in.Registry, " $\\\r\n\t") {
		return fmt.Errorf("%w: 镜像仓库须为域名和可选端口", ErrInvalid)
	}
	if (in.CAMode != "generate" && in.CAMode != "upload") || (in.CAMode == "generate" && (in.CACert != "" || in.CAKey != "")) {
		return fmt.Errorf("%w: 请选择生成或上传 CA", ErrInvalid)
	}
	return nil
}

// All shared configuration and the administrator commit together. bind pins the
// local deployment to one database before commit; a crash can only require a retry
// against that same database. No replica can replace an initialized deployment.
func (s *Store) initialize(ctx context.Context, in SetupRequest, ca *pki.Authority, encoded string, bind func() error) error {
	return s.Write(ctx, func(tx pgx.Tx) error {
		if err := s.migrate(ctx, tx); err != nil {
			return fmt.Errorf("%w: 数据库须为空库或地址池一致的 V2 库", ErrInvalid)
		}
		var used bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM administrator) OR EXISTS(SELECT 1 FROM deployment) OR EXISTS(SELECT 1 FROM devices) OR EXISTS(SELECT 1 FROM nodes) OR EXISTS(SELECT 1 FROM settings WHERE key='ca_fingerprint')`).Scan(&used); err != nil {
			return err
		}
		if used {
			return ErrConflict
		}
		if _, err := tx.Exec(ctx, `INSERT INTO deployment(id,database_url,public_url,registry,ca_cert,ca_key) VALUES(1,$1,$2,$3,$4,$5)`, in.DatabaseURL, in.PublicURL, in.Registry, ca.PEM, ca.SigningPEM()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO administrator(id,username,password_hash) VALUES(1,$1,$2)`, in.Username, encoded); err != nil {
			return err
		}
		if err := adminEvent(ctx, tx, in.Username, "admin.initialized", "deployment", struct{}{}); err != nil {
			return err
		}
		return bind()
	})
}

// LoadDeployment requires the new database-backed configuration; no environment
// or file CA fallback is accepted for an older deployment.
func LoadDeployment(ctx context.Context, databaseURL string) (*Server, error) {
	s, err := Open(ctx, databaseURL, model.DefaultPool)
	if err != nil {
		return nil, errors.New("database unavailable")
	}
	ok := false
	defer func() {
		if !ok {
			s.Pool.Close()
		}
	}()
	var valid bool
	if err = s.Pool.QueryRow(ctx, "SELECT count(*)=1 AND min(version)=2 FROM schema_version").Scan(&valid); err != nil || !valid {
		return nil, errors.New("database is not initialized V2")
	}
	var network, publicURL, registry, caCert, caKey string
	err = s.Pool.QueryRow(ctx, `SELECT s.value,d.public_url,d.registry,d.ca_cert,d.ca_key FROM deployment d JOIN settings s ON s.key='network' WHERE d.id=1 AND EXISTS(SELECT 1 FROM administrator)`).Scan(&network, &publicURL, &registry, &caCert, &caKey)
	if err != nil {
		return nil, errors.New("database deployment setup is incomplete")
	}
	// Reuse validation without retaining the database password in the API server.
	check := SetupRequest{Mode: "create", Username: "validation", Password: "validation password", DatabaseURL: databaseURL, Network: network, PublicURL: publicURL, Registry: registry, CAMode: "upload"}
	if err = check.normalize(); err != nil {
		return nil, errors.New("invalid stored deployment configuration")
	}
	n, err := parseNetwork(network)
	if err != nil {
		return nil, err
	}
	s.Network = n
	ca, err := pki.Parse([]byte(caCert), []byte(caKey))
	if err != nil || ca.ValidatePool(n) != nil {
		return nil, errors.New("invalid stored deployment CA")
	}
	ok = true
	return &Server{Store: s, CA: ca, PublicURL: publicURL, Registry: registry}, nil
}

func (s *Store) connectDeployment(ctx context.Context, in SetupRequest, bind func() error) error {
	var encoded string
	if err := s.Pool.QueryRow(ctx, "SELECT password_hash FROM administrator WHERE username=$1", in.Username).Scan(&encoded); err != nil || !passwordOK(encoded, in.Password) {
		return ErrUnauthorized
	}
	return s.Write(ctx, func(tx pgx.Tx) error {
		var current string
		if err := tx.QueryRow(ctx, "SELECT password_hash FROM administrator WHERE username=$1", in.Username).Scan(&current); err != nil || current != encoded {
			return ErrUnauthorized
		}
		if err := adminEvent(ctx, tx, in.Username, "control.connected", "deployment", struct{}{}); err != nil {
			return err
		}
		return bind()
	})
}
