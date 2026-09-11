package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
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
	root := command()
	if err := root.ExecuteContext(ctx); err != nil {
		asJSON, _ := root.PersistentFlags().GetBool("json")
		if asJSON {
			if root.Annotations["response_written"] != "true" {
				_ = json.NewEncoder(os.Stdout).Encode(model.NewResult("request_validation_failed", "cli", rand.Text(), nil))
			}
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(exitCode(err))
	}
}
func exitCode(err error) int {
	var business *model.BusinessError
	if !errors.As(err, &business) {
		return 2
	}
	code := model.Code(err)
	if model.IsCode(err, "local_rpc_timeout", "local_control_timeout", "local_storage_failed", "operation_pending", "operation_expired", "operation_not_found") {
		return 4
	}
	if model.HTTPStatus(code) >= 500 {
		return 3
	}
	return 2
}
func command() *cobra.Command {
	var dir string
	var asJSON bool
	var commandID string
	root := &cobra.Command{Use: "nlroom-cli", Short: "NodeLane Room CLI for automation and diagnostics", SilenceUsage: true, SilenceErrors: true, Annotations: map[string]string{}, Version: model.ClientVersion + " (Nebula " + model.NebulaVersion + ")"}
	root.PersistentFlags().StringVar(&dir, "state-dir", platform.DefaultDir(), "Agent state directory")
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "Print machine-readable JSON")
	root.PersistentFlags().StringVar(&commandID, "command-id", "", "Reuse the original operation ID for recovery")
	print := func(cmd *cobra.Command, v any) error {
		e := json.NewEncoder(cmd.OutOrStdout())
		e.SetIndent("", "  ")
		return e.Encode(v)
	}
	call := func(cmd *cobra.Command, req localapi.Request) error {
		req.CommandID = commandID
		var out model.Result
		err := localapi.Call(cmd.Context(), dir, req, &out)
		if err != nil && out.Contract == "" {
			out = model.NewResult(model.Code(err), "cli", rand.Text(), nil)
			var failure *localapi.Error
			if errors.As(err, &failure) && failure.Result != nil {
				out = *failure.Result
			}
		}
		if asJSON {
			root.Annotations["response_written"] = "true"
			if e := print(cmd, out); e != nil {
				return e
			}
		} else if err == nil {
			if e := print(cmd, out.Data); e != nil {
				return e
			}
		}
		if err == nil && req.Action == "get-operation" {
			var op model.Operation
			if json.Unmarshal(out.Data, &op) == nil {
				switch op.State {
				case "submitting", "pending", "reconciling", "unresolved":
					return model.Failure("operation_pending")
				case "rejected":
					if op.Result != nil {
						return model.Failure(op.Result.Code)
					}
				}
			}
		}
		return err
	}
	var server, name string
	init := &cobra.Command{Use: "init", Short: "Create a guest account with a local device credential", RunE: func(cmd *cobra.Command, _ []string) error {
		return call(cmd, localapi.Request{Action: "init", Server: server, Name: name})
	}}
	init.Flags().StringVar(&server, "server", "", "Control HTTPS origin")
	init.Flags().StringVar(&name, "name", "", "Device nickname")
	_ = init.MarkFlagRequired("server")
	_ = init.MarkFlagRequired("name")
	root.AddCommand(init)
	account := &cobra.Command{Use: "account", Short: "Guest upgrade and account login; open the returned URL in your browser"}
	var loginServer, loginName string
	login := &cobra.Command{Use: "login", Args: cobra.NoArgs, Short: "Sign in to an account; switching discards access to an unlinked guest", RunE: func(cmd *cobra.Command, _ []string) error {
		return call(cmd, localapi.Request{Action: "account-login", Server: loginServer, Name: loginName})
	}}
	login.Flags().StringVar(&loginServer, "server", "https://room.nodelane.net", "Control origin for an unconfigured device")
	login.Flags().StringVar(&loginName, "name", "Player", "Display name for a new account")
	account.AddCommand(login)
	for _, action := range []string{"link", "poll", "cancel", "logout", "devices", "status"} {
		account.AddCommand(&cobra.Command{Use: action, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return call(cmd, localapi.Request{Action: "account-" + action})
		}})
	}
	var occupancy model.TakeoverRequest
	takeover := &cobra.Command{Use: "takeover", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		b, _ := json.Marshal(occupancy)
		return call(cmd, localapi.Request{Action: "account-takeover", Body: b})
	}}
	takeover.Flags().StringVar(&occupancy.RoomID, "room", "", "Observed occupied room")
	takeover.Flags().StringVar(&occupancy.DeviceID, "device", "", "Observed occupied device")
	takeover.Flags().Int64Var(&occupancy.ExpectedRevision, "expected-revision", 0, "Observed membership revision")
	for _, flag := range []string{"room", "device", "expected-revision"} {
		_ = takeover.MarkFlagRequired(flag)
	}
	account.AddCommand(takeover)
	root.AddCommand(account)
	root.AddCommand(&cobra.Command{Use: "games", Short: "List server games and their configured ports", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if asJSON {
			return call(cmd, localapi.Request{Action: "games"})
		}
		var games []model.Game
		if err := localapi.Call(cmd.Context(), dir, localapi.Request{Action: "games"}, &games); err != nil {
			return err
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
	var expectedRevision, gameRevision int64
	room.PersistentFlags().StringVar(&selected, "room", "", "Explicit room ID (otherwise current room)")
	room.PersistentFlags().Int64Var(&expectedRevision, "expected-revision", 0, "Revision shown by the latest room snapshot")
	var roomName, gameName string
	create := &cobra.Command{Use: "create", Short: "Create a room and invitation", RunE: func(cmd *cobra.Command, _ []string) error {
		b, _ := json.Marshal(model.RoomRequest{Name: roomName, Game: gameName, ExpectedGameRevision: gameRevision})
		return call(cmd, localapi.Request{Action: "create", Body: b})
	}}
	create.Flags().StringVar(&roomName, "name", "", "Room name")
	create.Flags().StringVar(&gameName, "game", "custom", "Game ID from games; all network permissions are server-configured")
	_ = create.MarkFlagRequired("name")
	create.Flags().Int64Var(&gameRevision, "game-revision", 0, "Revision from the selected game")
	_ = create.MarkFlagRequired("game-revision")
	room.AddCommand(create)
	room.AddCommand(&cobra.Command{Use: "join <invitation>", Args: cobra.ExactArgs(1), Short: "Join and connect to a room", RunE: func(cmd *cobra.Command, args []string) error {
		b, _ := json.Marshal(model.JoinRequest{Code: args[0]})
		return call(cmd, localapi.Request{Action: "join", Body: b})
	}})
	for _, action := range []string{"members", "manage", "invite", "invite-info", "invite-revoke", "owner-join", "leave", "close"} {
		room.AddCommand(&cobra.Command{Use: action, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			b, _ := json.Marshal(model.MemberRequest{ExpectedRevision: expectedRevision})
			return call(cmd, localapi.Request{Action: action, Room: selected, Body: b})
		}})
	}
	for _, action := range []string{"kick", "transfer"} {
		room.AddCommand(&cobra.Command{Use: action + " <device-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			b, _ := json.Marshal(model.MemberRequest{DeviceID: args[0], ExpectedRevision: expectedRevision})
			return call(cmd, localapi.Request{Action: action, Room: selected, Body: b})
		}})
	}

	root.AddCommand(room)
	for _, action := range []string{"network-stop", "network-retry", "rooms"} {
		root.AddCommand(&cobra.Command{Use: action, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return call(cmd, localapi.Request{Action: action}) }})
	}
	root.AddCommand(&cobra.Command{Use: "get-operation <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd, localapi.Request{Action: "get-operation", Target: args[0]})
	}})
	var watch bool
	status := &cobra.Command{Use: "status", Short: "Show control and game connection states", RunE: func(cmd *cobra.Command, _ []string) error {
		for {
			var s model.Status
			if asJSON {
				if err := call(cmd, localapi.Request{Action: "status"}); err != nil {
					return err
				}
				if !watch {
					return nil
				}
			} else {
				if err := localapi.Call(cmd.Context(), dir, localapi.Request{Action: "status"}, &s); err != nil {
					return err
				}
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
		if asJSON {
			return call(cmd, localapi.Request{Action: "status"})
		}
		var s model.Status
		if err := localapi.Call(cmd.Context(), dir, localapi.Request{Action: "status"}, &s); err != nil {
			return err
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
