package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nodelane/nodelane-room/internal/control"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func main() {
	if err := command().Execute(); err != nil {
		os.Exit(1)
	}
}

func command() *cobra.Command {
	var stateDir string
	root := &cobra.Command{Use: "nodelane-server", Short: "NodeLane Room control service", SilenceUsage: true, Version: model.ControlVersion + " (protocol v2; Nebula " + model.NebulaVersion + ")"}
	root.PersistentFlags().StringVar(&stateDir, "state-dir", "/var/lib/nodelane-control", "Private control instance state directory")
	var listen, tlsCert, tlsKey, releaseDir, geoIPPath, geoIPURL, adminPath string
	var behindProxy bool
	serve := &cobra.Command{Use: "serve", Short: "Run the setup page and control API", RunE: func(cmd *cobra.Command, _ []string) error {
		host, _, err := net.SplitHostPort(listen)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if tlsCert == "" && !behindProxy && (ip == nil || !ip.IsLoopback()) {
			return errors.New("public HTTP requires --behind-proxy or --tls-cert and --tls-key")
		}
		if (tlsCert == "") != (tlsKey == "") {
			return errors.New("--tls-cert and --tls-key must be provided together")
		}
		if err = platform.SecureDir(stateDir); err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		entry, err := control.ConfigureAdminPath(stateDir, adminPath)
		if err != nil {
			return err
		}
		var geo *control.GeoIP
		if geoIPPath != "" {
			geo, err = control.OpenGeoIP(geoIPPath)
			if err != nil {
				return fmt.Errorf("load GeoIP database: %w", err)
			}
			defer geo.Close()
		} else {
			var stopGeo func()
			geo, stopGeo, err = control.AutoGeoIP(ctx, stateDir, geoIPURL, log)
			if err != nil {
				return err
			}
			defer stopGeo()
		}
		deployment := &control.Deployment{StateDir: stateDir, AdminPath: entry, ReleaseDir: releaseDir, BehindProxy: behindProxy, Log: log, GeoIP: geo}
		// Keep the pool alive until HTTP shutdown drains in-flight requests.
		runCtx, stop := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); deployment.Run(runCtx) }()
		defer func() { stop(); <-done }()
		srv := &http.Server{Addr: listen, Handler: deployment.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
		shutdown := make(chan struct{})
		go func() {
			defer close(shutdown)
			<-ctx.Done()
			c, finish := context.WithTimeout(context.Background(), 10*time.Second)
			defer finish()
			_ = srv.Shutdown(c)
		}()
		log.Info("control API listening", "address", listen, "nebula", model.NebulaVersion)
		if tlsCert != "" {
			err = srv.ListenAndServeTLS(tlsCert, tlsKey)
		} else {
			err = srv.ListenAndServe()
		}
		cancel()
		<-shutdown
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}}
	serve.Flags().StringVar(&listen, "listen", "127.0.0.1:8080", "HTTP listen address")
	serve.Flags().StringVar(&tlsCert, "tls-cert", "", "HTTPS certificate")
	serve.Flags().StringVar(&tlsKey, "tls-key", "", "HTTPS private key")
	serve.Flags().BoolVar(&behindProxy, "behind-proxy", false, "Allow HTTP behind a TLS reverse proxy")
	serve.Flags().StringVar(&releaseDir, "release-dir", "/opt/nodelane/releases", "Read-only installer and release directory")
	serve.Flags().StringVar(&adminPath, "admin-path", os.Getenv("NODELANE_ADMIN_PATH"), "Private admin entry; generated and persisted when unset")
	serve.Flags().StringVar(&geoIPPath, "geoip-db", os.Getenv("NODELANE_GEOIP_DB"), "Optional local MMDB override; disables automatic downloads")
	geoIPURL = os.Getenv("NODELANE_GEOIP_URL")
	if geoIPURL == "" {
		geoIPURL = control.DefaultGeoIPURL
	}
	serve.Flags().StringVar(&geoIPURL, "geoip-url", geoIPURL, "Automatic HTTPS .mmdb.gz source; {month} expands to YYYY-MM")
	root.AddCommand(serve)
	admin := &cobra.Command{Use: "admin", Short: "Initialize or recover the administrator"}
	admin.AddCommand(&cobra.Command{Use: "path", Short: "Show this instance's private admin entry", RunE: func(cmd *cobra.Command, _ []string) error {
		path, err := control.AdminPath(stateDir)
		if err != nil {
			return errors.New("admin entry unavailable; start this instance first")
		}
		fmt.Fprintln(cmd.OutOrStdout(), path)
		return nil
	}})
	admin.AddCommand(&cobra.Command{Use: "bootstrap", RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
		defer cancel()
		token, err := control.BootstrapCode(ctx, stateDir)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), token)
		return nil
	}})
	admin.AddCommand(&cobra.Command{Use: "reset-password", RunE: func(cmd *cobra.Command, _ []string) error {
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("interactive terminal required: %w", err)
		}
		defer tty.Close()
		fmt.Fprint(tty, "New administrator password (12–128 bytes): ")
		password, err := term.ReadPassword(int(tty.Fd()))
		fmt.Fprintln(tty)
		if err != nil {
			return err
		}
		defer clear(password)
		db, err := control.DatabaseURL(stateDir)
		if err != nil {
			return errors.New("database locator unavailable; complete web setup first")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
		defer cancel()
		s, err := control.Open(ctx, db, model.DefaultPool)
		if err != nil {
			return errors.New("database unavailable")
		}
		defer s.Pool.Close()
		if err = s.ResetAdminPassword(ctx, strings.TrimRight(string(password), "\r\n")); err != nil {
			return errors.New("password reset failed")
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Password reset; all administrator sessions revoked.")
		return nil
	}})
	root.AddCommand(admin)
	return root
}
