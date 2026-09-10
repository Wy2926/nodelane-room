package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/control"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"strings"
)

func main() {
	if err := command().Execute(); err != nil {
		os.Exit(1)
	}
}
func command() *cobra.Command {
	var db, network string
	root := &cobra.Command{Use: "nodelane-server", Short: "NodeLane Room control service", SilenceUsage: true}
	root.PersistentFlags().StringVar(&db, "database-url", os.Getenv("DATABASE_URL"), "PostgreSQL connection string (prefer DATABASE_URL)")
	root.PersistentFlags().StringVar(&network, "network", model.DefaultPool, "Overlay IPv4 pool")
	open := func(ctx context.Context) (*control.Store, error) {
		if db == "" {
			return nil, errors.New("DATABASE_URL is required")
		}
		return control.Open(ctx, db, network)
	}
	root.AddCommand(&cobra.Command{Use: "migrate", Short: "Apply database migrations", RunE: func(cmd *cobra.Command, _ []string) error {
		s, e := open(cmd.Context())
		if e != nil {
			return e
		}
		defer s.Pool.Close()
		return s.Migrate(cmd.Context())
	}})
	var caDir string
	ca := &cobra.Command{Use: "ca", Short: "Manage the Nebula CA"}
	init := &cobra.Command{Use: "init", Short: "Create a CA without overwriting existing keys", RunE: func(cmd *cobra.Command, _ []string) error {
		n, e := netip.ParsePrefix(network)
		if e != nil {
			return e
		}
		a, e := pki.Generate(n)
		if e != nil {
			return e
		}
		return a.Save(caDir)
	}}
	init.Flags().StringVar(&caDir, "dir", "secrets", "CA output directory")
	ca.AddCommand(init)
	root.AddCommand(ca)
	var listen, caCert, caKey, tlsCert, tlsKey, publicURL, releaseDir string
	var behindProxy bool
	serve := &cobra.Command{Use: "serve", Short: "Run the control API", RunE: func(cmd *cobra.Command, _ []string) error {
		if err := client.ValidateURL(publicURL); err != nil {
			return fmt.Errorf("PUBLIC_URL: %w", err)
		}
		host, _, e := net.SplitHostPort(listen)
		if e != nil {
			return e
		}
		ip := net.ParseIP(host)
		if tlsCert == "" && !behindProxy && (ip == nil || !ip.IsLoopback()) {
			return errors.New("public HTTP requires --behind-proxy or --tls-cert and --tls-key")
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		s, e := open(ctx)
		if e != nil {
			return e
		}
		defer s.Pool.Close()
		var stored string
		var validSchema bool
		if e = s.Pool.QueryRow(ctx, "SELECT count(*)=1 AND min(version)=2 FROM schema_version").Scan(&validSchema); e != nil || !validSchema {
			return errors.New("control service requires an initialized V2 database; use migrate on an empty database")
		}
		if e = s.Pool.QueryRow(ctx, "SELECT value FROM settings WHERE key='network'").Scan(&stored); e != nil {
			return fmt.Errorf("run migrate before serve: %w", e)
		}
		if stored != network {
			return errors.New("network differs from migrated database")
		}
		a, e := pki.Load(caCert, caKey)
		if e != nil {
			return e
		}
		f, _ := a.Certificate.Fingerprint()
		var pinned string
		if _, e = s.Pool.Exec(ctx, "INSERT INTO settings(key,value) VALUES('ca_fingerprint',$1) ON CONFLICT DO NOTHING", f); e != nil {
			return e
		}
		if e = s.Pool.QueryRow(ctx, "SELECT value FROM settings WHERE key='ca_fingerprint'").Scan(&pinned); e != nil {
			return e
		}
		if pinned != f {
			return errors.New("CA differs from other control replicas")
		}
		log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		api := &control.Server{Store: s, CA: a, Log: log, PublicURL: publicURL, ReleaseDir: releaseDir}
		srv := &http.Server{Addr: listen, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				if e := s.Sweep(ctx); e != nil && ctx.Err() == nil {
					log.Error("maintenance failed", "error", e)
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
		go func() {
			<-ctx.Done()
			c, done := context.WithTimeout(context.Background(), 10*time.Second)
			defer done()
			_ = srv.Shutdown(c)
		}()
		log.Info("control API listening", "address", listen, "nebula", model.NebulaVersion)
		if tlsCert != "" {
			e = srv.ListenAndServeTLS(tlsCert, tlsKey)
		} else {
			e = srv.ListenAndServe()
		}
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return e
	}}
	serve.Flags().StringVar(&listen, "listen", "127.0.0.1:8080", "HTTP listen address")
	serve.Flags().StringVar(&caCert, "ca-cert", "secrets/ca.crt", "Nebula CA certificate")
	serve.Flags().StringVar(&caKey, "ca-key", "secrets/ca.key", "Nebula CA signing key")
	serve.Flags().StringVar(&tlsCert, "tls-cert", "", "HTTPS certificate")
	serve.Flags().StringVar(&tlsKey, "tls-key", "", "HTTPS private key")
	serve.Flags().BoolVar(&behindProxy, "behind-proxy", false, "Allow HTTP on a private network behind a TLS reverse proxy")
	serve.Flags().StringVar(&publicURL, "public-url", os.Getenv("PUBLIC_URL"), "Public HTTPS origin for web and node enrollment")
	serve.Flags().StringVar(&releaseDir, "release-dir", "/opt/nodelane/releases", "Read-only installer and release directory")
	root.AddCommand(serve)
	admin := &cobra.Command{Use: "admin", Short: "Initialize or recover the administrator"}
	admin.AddCommand(&cobra.Command{Use: "bootstrap", RunE: func(cmd *cobra.Command, _ []string) error {
		s, e := open(cmd.Context())
		if e != nil {
			return e
		}
		defer s.Pool.Close()
		token, e := s.AdminBootstrap(cmd.Context())
		if e != nil {
			return e
		}
		fmt.Fprintln(cmd.OutOrStdout(), token)
		return nil
	}})
	admin.AddCommand(&cobra.Command{Use: "reset-password", RunE: func(cmd *cobra.Command, _ []string) error {
		tty, e := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if e != nil {
			return fmt.Errorf("interactive terminal required: %w", e)
		}
		defer tty.Close()
		fmt.Fprint(tty, "New administrator password (12–128 bytes): ")
		password, e := term.ReadPassword(int(tty.Fd()))
		fmt.Fprintln(tty)
		if e != nil {
			return e
		}
		defer clear(password)
		s, e := open(cmd.Context())
		if e != nil {
			return e
		}
		defer s.Pool.Close()
		if e = s.ResetAdminPassword(cmd.Context(), strings.TrimRight(string(password), "\r\n")); e != nil {
			return e
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Password reset; all administrator sessions revoked.")
		return nil
	}})
	root.AddCommand(admin)
	return root
}
