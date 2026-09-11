package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/nodelane/nodelane-room/internal/localapi"
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
	root := &cobra.Command{Use: "nlroom-cli", Short: "NodeLane Room CLI for automation and diagnostics", SilenceUsage: true, Version: model.Version + " (Nebula " + model.NebulaVersion + ")"}
	root.PersistentFlags().StringVar(&dir, "state-dir", platform.DefaultDir(), "Agent state directory")
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "Print machine-readable JSON")
	print := func(cmd *cobra.Command, v any) error {
		e := json.NewEncoder(cmd.OutOrStdout())
		e.SetIndent("", "  ")
		return e.Encode(v)
	}
	call := func(cmd *cobra.Command, req localapi.Request) error {
		var out json.RawMessage
		if err := localapi.Call(cmd.Context(), dir, req, &out); err != nil {
			return err
		}
		return print(cmd, out)
	}
	var server, name string
	init := &cobra.Command{Use: "init", Short: "Create and register this device", RunE: func(cmd *cobra.Command, _ []string) error {
		return call(cmd, localapi.Request{Action: "init", Server: server, Name: name})
	}}
	init.Flags().StringVar(&server, "server", "", "Control HTTPS origin")
	init.Flags().StringVar(&name, "name", "", "Device nickname")
	_ = init.MarkFlagRequired("server")
	_ = init.MarkFlagRequired("name")
	root.AddCommand(init)
	root.AddCommand(&cobra.Command{Use: "games", Short: "List server games and their configured ports", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		var games []model.Game
		if err := localapi.Call(cmd.Context(), dir, localapi.Request{Action: "games"}, &games); err != nil {
			return err
		}
		if asJSON {
			return print(cmd, games)
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tGAME\tPORTS")
		for _, g := range games {
			var ports []string
			for _, p := range g.Ports {
				value := fmt.Sprintf("%s/%d", p.Protocol, p.Port)
				if p.PortEnd > p.Port {
					value += fmt.Sprintf("-%d", p.PortEnd)
				}
				ports = append(ports, value)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", g.ID, g.Name, strings.Join(ports, ", "))
		}
		return w.Flush()
	}})
	room := &cobra.Command{Use: "room", Short: "Manage game rooms"}
	var selected string
	room.PersistentFlags().StringVar(&selected, "room", "", "Explicit room ID (otherwise current room)")
	var roomName, gameName string
	create := &cobra.Command{Use: "create", Short: "Create a room and invitation", RunE: func(cmd *cobra.Command, _ []string) error {
		b, _ := json.Marshal(model.RoomRequest{Name: roomName, Game: gameName})
		return call(cmd, localapi.Request{Action: "create", Body: b})
	}}
	create.Flags().StringVar(&roomName, "name", "", "Room name")
	create.Flags().StringVar(&gameName, "game", "custom", "Game ID from games; all network permissions are server-configured")
	_ = create.MarkFlagRequired("name")
	room.AddCommand(create)
	room.AddCommand(&cobra.Command{Use: "join <invitation>", Args: cobra.ExactArgs(1), Short: "Join and connect to a room", RunE: func(cmd *cobra.Command, args []string) error {
		b, _ := json.Marshal(model.JoinRequest{Code: args[0]})
		return call(cmd, localapi.Request{Action: "join", Body: b})
	}})
	for _, action := range []string{"members", "invite", "leave", "close"} {
		room.AddCommand(&cobra.Command{Use: action, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return call(cmd, localapi.Request{Action: action, Room: selected})
		}})
	}
	for _, action := range []string{"kick", "transfer"} {
		room.AddCommand(&cobra.Command{Use: action + " <device-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			b, _ := json.Marshal(model.MemberRequest{DeviceID: args[0]})
			return call(cmd, localapi.Request{Action: action, Room: selected, Body: b})
		}})
	}

	root.AddCommand(room)
	var watch bool
	status := &cobra.Command{Use: "status", Short: "Show control and game connection states", RunE: func(cmd *cobra.Command, _ []string) error {
		for {
			var s model.Status
			if err := localapi.Call(cmd.Context(), dir, localapi.Request{Action: "status"}, &s); err != nil {
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
		if err := localapi.Call(cmd.Context(), dir, localapi.Request{Action: "status"}, &s); err != nil {
			return err
		}
		if asJSON {
			return print(cmd, s.Peers)
		}
		peerTable(cmd, s.Peers)
		return nil
	}})
	root.AddCommand(&cobra.Command{Use: "ping <member>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd, localapi.Request{Action: "ping", Target: args[0]})
	}})
	root.AddCommand(&cobra.Command{Use: "doctor", Short: "Inspect driver, permissions, lease and connectivity state", RunE: func(cmd *cobra.Command, _ []string) error { return call(cmd, localapi.Request{Action: "doctor"}) }})
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
