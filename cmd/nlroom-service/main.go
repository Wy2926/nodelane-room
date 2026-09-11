package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/nodelane/nodelane-room/internal/agent"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
	"github.com/spf13/cobra"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := command().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func command() *cobra.Command {
	var dir string
	root := &cobra.Command{Use: "nlroom-service", Short: "NodeLane Room background networking service", SilenceUsage: true, Version: model.ClientVersion + " (Nebula " + model.NebulaVersion + ")"}
	root.PersistentFlags().StringVar(&dir, "state-dir", platform.DefaultDir(), "Agent state directory")
	daemon := &cobra.Command{Use: "daemon", Short: "Run the background agent (Administrator on Windows)", RunE: func(cmd *cobra.Command, _ []string) error {
		run := func(ctx context.Context) error {
			if err := platform.EnsureForegroundOwner(dir); err != nil {
				return err
			}
			f, err := os.OpenFile(filepath.Join(dir, "agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			defer f.Close()
			log := slog.New(slog.NewJSONHandler(f, nil))
			r, err := agent.New(dir, log)
			if err != nil {
				return err
			}
			return r.Serve(ctx)
		}
		if handled, err := platform.RunService(run); handled || err != nil {
			return err
		}
		return run(cmd.Context())
	}}
	root.AddCommand(daemon)
	service := &cobra.Command{Use: "service", Short: "Manage the Windows service"}
	var ownerSID string
	install := &cobra.Command{Use: "install", RunE: func(_ *cobra.Command, _ []string) error { return platform.Install(dir, ownerSID) }}
	install.Flags().StringVar(&ownerSID, "owner-sid", "", "SID allowed to control the service (default: installing user)")
	service.AddCommand(install, &cobra.Command{Use: "uninstall", RunE: func(_ *cobra.Command, _ []string) error { return platform.Uninstall(dir) }})
	root.AddCommand(service)
	return root
}
