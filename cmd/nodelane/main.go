package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/nodelane/nodelane-room/internal/agent"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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
	var asJSON bool
	root := &cobra.Command{Use: "nodelane", Short: "Private game rooms powered by Nebula", SilenceUsage: true, Version: model.Version + " (Nebula " + model.NebulaVersion + ")"}
	root.PersistentFlags().StringVar(&dir, "state-dir", platform.DefaultDir(), "Agent state directory")
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "Print machine-readable JSON")
	print := func(cmd *cobra.Command, v any) error {
		e := json.NewEncoder(cmd.OutOrStdout())
		e.SetIndent("", "  ")
		return e.Encode(v)
	}
	call := func(cmd *cobra.Command, req agent.Request) error {
		var out json.RawMessage
		if err := agent.LocalCall(cmd.Context(), dir, req, &out); err != nil {
			return err
		}
		return print(cmd, out)
	}
	var server, name string
	init := &cobra.Command{Use: "init", Short: "Create and register this device", RunE: func(cmd *cobra.Command, _ []string) error {
		return call(cmd, agent.Request{Action: "init", Server: server, Name: name})
	}}
	init.Flags().StringVar(&server, "server", "", "Control HTTPS origin")
	init.Flags().StringVar(&name, "name", "", "Device nickname")
	_ = init.MarkFlagRequired("server")
	_ = init.MarkFlagRequired("name")
	root.AddCommand(init)
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
	room := &cobra.Command{Use: "room", Short: "Manage game rooms"}
	var selected string
	room.PersistentFlags().StringVar(&selected, "room", "", "Explicit room ID (otherwise current room)")
	var roomName, gameName string
	create := &cobra.Command{Use: "create", Short: "Create a room and invitation", RunE: func(cmd *cobra.Command, _ []string) error {
		b, _ := json.Marshal(model.RoomRequest{Name: roomName, Game: gameName})
		return call(cmd, agent.Request{Action: "create", Body: b})
	}}
	create.Flags().StringVar(&roomName, "name", "", "Room name")
	create.Flags().StringVar(&gameName, "game", "minecraft-java", "minecraft-java or custom")
	_ = create.MarkFlagRequired("name")
	room.AddCommand(create)
	room.AddCommand(&cobra.Command{Use: "join <invitation>", Args: cobra.ExactArgs(1), Short: "Join and connect to a room", RunE: func(cmd *cobra.Command, args []string) error {
		b, _ := json.Marshal(model.JoinRequest{Code: args[0]})
		return call(cmd, agent.Request{Action: "join", Body: b})
	}})
	for _, action := range []string{"members", "invite", "leave", "close"} {
		room.AddCommand(&cobra.Command{Use: action, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return call(cmd, agent.Request{Action: action, Room: selected})
		}})
	}
	for _, action := range []string{"kick", "transfer"} {
		room.AddCommand(&cobra.Command{Use: action + " <device-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			b, _ := json.Marshal(model.MemberRequest{DeviceID: args[0]})
			return call(cmd, agent.Request{Action: action, Room: selected, Body: b})
		}})
	}
	room.AddCommand(&cobra.Command{Use: "port <tcp|udp>/<port>", Short: "Expose an additional game port while connected", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		parts := strings.Split(args[0], "/")
		if len(parts) != 2 {
			return fmt.Errorf("use tcp/25565 or udp/27015")
		}
		port, err := strconv.ParseUint(parts[1], 10, 16)
		if err != nil {
			return err
		}
		b, _ := json.Marshal(model.EndpointRequest{Protocol: parts[0], Port: uint16(port)})
		return call(cmd, agent.Request{Action: "port", Body: b})
	}})
	root.AddCommand(room)
	var watch bool
	status := &cobra.Command{Use: "status", Short: "Show control and game connection states", RunE: func(cmd *cobra.Command, _ []string) error {
		for {
			var s model.Status
			if err := agent.LocalCall(cmd.Context(), dir, agent.Request{Action: "status"}, &s); err != nil {
				return err
			}
			if asJSON {
				if err := print(cmd, s); err != nil {
					return err
				}
			} else {
				if watch && term.IsTerminal(int(os.Stdout.Fd())) {
					fmt.Fprint(cmd.OutOrStdout(), "\x1b[H\x1b[2J")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Control: %s   Nebula: %s   IP: %s\n", s.Control, s.Engine, s.IP)
				if s.Room != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Room: %s (%s)\n", s.Room.Name, s.Room.ID)
				}
				if s.Error != "" {
					fmt.Fprintln(cmd.OutOrStdout(), "Detail:", s.Error)
				}
				peerTable(cmd, s.Peers)
			}
			if !watch {
				return nil
			}
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-cmd.Context().Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}
	}}
	status.Flags().BoolVar(&watch, "watch", false, "Refresh every two seconds")
	root.AddCommand(status)
	root.AddCommand(&cobra.Command{Use: "peers", Short: "Show peer paths and measured latency", RunE: func(cmd *cobra.Command, _ []string) error {
		var s model.Status
		if err := agent.LocalCall(cmd.Context(), dir, agent.Request{Action: "status"}, &s); err != nil {
			return err
		}
		if asJSON {
			return print(cmd, s.Peers)
		}
		peerTable(cmd, s.Peers)
		return nil
	}})
	root.AddCommand(&cobra.Command{Use: "ping <member>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd, agent.Request{Action: "ping", Target: args[0]})
	}})
	root.AddCommand(&cobra.Command{Use: "doctor", Short: "Inspect driver, permissions, lease and connectivity state", RunE: func(cmd *cobra.Command, _ []string) error { return call(cmd, agent.Request{Action: "doctor"}) }})
	return root
}
func peerTable(cmd *cobra.Command, peers []model.Peer) {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVIRTUAL IP\tPATH\tRTT\tLOSS")
	for _, p := range peers {
		rtt, loss := "—", "—"
		if p.RTTMillis != nil {
			rtt = fmt.Sprintf("%.1f ms", *p.RTTMillis)
		}
		if p.LossPercent != nil {
			loss = fmt.Sprintf("%.0f%%", *p.LossPercent)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.IP, p.Mode, rtt, loss)
	}
	_ = w.Flush()
}
