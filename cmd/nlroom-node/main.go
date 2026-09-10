package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nodelane/nodelane-room/internal/agent"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/nodehost"
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
	var dir, server, configPath string
	var asJSON bool
	root := &cobra.Command{Use: "nlroom-node", Short: "NodeLane Room 基础设施节点", SilenceUsage: true, Version: model.Version + " (protocol v2; Nebula " + model.NebulaVersion + ")"}
	root.PersistentFlags().StringVar(&dir, "state-dir", "/var/lib/nlroom-node", "Private node state directory")
	root.PersistentFlags().StringVar(&server, "server", "", "Control HTTPS origin")
	root.PersistentFlags().StringVar(&configPath, "config-file", "/etc/nlroom-node/config.json", "Native installation configuration")
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "Print JSON")
	printJSON := func(cmd *cobra.Command, v any) error {
		e := json.NewEncoder(cmd.OutOrStdout())
		e.SetIndent("", "  ")
		return e.Encode(v)
	}
	call := func(cmd *cobra.Command, action string, body any, out any) error {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		err = agent.LocalCall(cmd.Context(), dir, agent.Request{Action: action, Body: b}, out)
		if err != nil && strings.Contains(err.Error(), "cannot reach NodeLane service") {
			return errors.New("无法连接节点管理进程；原生部署运行 nlroom-node service start，容器部署检查 Compose/1Panel 运行状态和身份目录挂载")
		}
		return err
	}
	var health string
	var port int
	run := &cobra.Command{Use: "run", Short: "Run the foreground node and local management socket", RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := nodehost.ReadConfig(configPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if server == "" {
			server = cfg.Server
		}
		log, buffer := agent.NodeLogger()
		r, err := agent.New(dir, log)
		if err != nil {
			return err
		}
		if err = r.ConfigureNode(strings.TrimRight(server, "/"), port); err != nil {
			return err
		}
		r.SetLogBuffer(buffer)
		listener, err := net.Listen("tcp", health)
		if err != nil {
			return err
		}
		h := &http.Server{Handler: r.HealthHandler(), ReadHeaderTimeout: 5 * time.Second}
		defer h.Close()
		go func() {
			if err := h.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("health listener failed", "error", err)
			}
		}()
		return r.Serve(cmd.Context())
	}}
	run.Flags().StringVar(&health, "health-listen", "127.0.0.1:9090", "Loopback health endpoint")
	run.Flags().IntVar(&port, "listen-port", 0, "Configured local UDP port (0 adopts initial authorized port)")
	root.AddCommand(run)
	var stdinKey bool
	enroll := &cobra.Command{Use: "enroll", Short: "Register with an administrator-issued temporary key", RunE: func(cmd *cobra.Command, _ []string) error {
		var status model.NodeLocalStatus
		if err := call(cmd, "status", nil, &status); err != nil {
			return err
		}
		key := ""
		if !status.Registered {
			// Recover a completed server-side binding before asking for another key.
			var recovered model.NodeLocalStatus
			if err := call(cmd, "enroll", map[string]string{"key": ""}, &recovered); err == nil {
				status = recovered
			}
		}
		if !status.Registered {
			if stdinKey {
				var err error
				key, err = nodehost.ReadSecret(cmd.InOrStdin())
				if err != nil {
					return err
				}
			} else {
				tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
				if err != nil {
					return errors.New("需要交互终端；自动化请通过 --key-stdin 安全传入密钥")
				}
				defer tty.Close()
				fmt.Fprint(tty, "请输入临时接入密钥（输入不回显）：")
				b, err := term.ReadPassword(int(tty.Fd()))
				fmt.Fprintln(tty)
				if err != nil {
					return err
				}
				key = strings.TrimSpace(string(b))
				clear(b)
			}
		}
		if err := call(cmd, "enroll", map[string]string{"key": key}, &status); err != nil {
			return err
		}
		if asJSON {
			return printJSON(cmd, status)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "节点已登记，正在启动数据面。")
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			if err := call(cmd, "status", nil, &status); err != nil {
				return err
			}
			if status.Engine == "running" {
				return printStatus(cmd, status)
			}
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case <-time.After(time.Second):
			}
		}
		_ = printStatus(cmd, status)
		return errors.New("登记已保存，数据面尚未就绪；运行 nlroom-node doctor 查看原因")
	}}
	enroll.Flags().BoolVar(&stdinKey, "key-stdin", false, "Read the enrollment key from stdin; never pass secrets as arguments")
	root.AddCommand(enroll)
	var watch bool
	status := &cobra.Command{Use: "status", Short: "Show current runtime state", RunE: func(cmd *cobra.Command, _ []string) error {
		for {
			var s model.NodeLocalStatus
			if err := call(cmd, "status", nil, &s); err != nil {
				return err
			}
			if asJSON {
				if err := printJSON(cmd, s); err != nil {
					return err
				}
			} else {
				if err := printStatus(cmd, s); err != nil {
					return err
				}
			}
			if !watch {
				return nil
			}
			select {
			case <-cmd.Context().Done():
				return nil
			case <-time.After(2 * time.Second):
			}
		}
	}}
	status.Flags().BoolVar(&watch, "watch", false, "Continuously refresh status")
	root.AddCommand(status)
	for _, action := range []string{"doctor", "restart"} {
		a := action
		root.AddCommand(&cobra.Command{Use: a, RunE: func(cmd *cobra.Command, _ []string) error {
			var out json.RawMessage
			if err := call(cmd, a, nil, &out); err != nil {
				return err
			}
			return printJSON(cmd, out)
		}})
	}
	conf := &cobra.Command{Use: "config", Short: "Show desired and applied configuration", RunE: func(cmd *cobra.Command, _ []string) error {
		var s model.NodeLocalStatus
		if err := call(cmd, "config", nil, &s); err != nil {
			return err
		}
		desired, applied := map[string]any{}, map[string]any{}
		if s.Node != nil {
			b, _ := json.Marshal(s.Node.Config())
			_ = json.Unmarshal(b, &desired)
		}
		if s.Report.AppliedConfig != nil {
			b, _ := json.Marshal(s.Report.AppliedConfig)
			_ = json.Unmarshal(b, &applied)
		}
		diff := map[string]any{}
		for name, value := range desired {
			if applied[name] != value {
				diff[name] = map[string]any{"desired": value, "applied": applied[name]}
			}
		}
		return printJSON(cmd, map[string]any{"desired": s.Node, "applied": s.Report.AppliedConfig, "applied_revision": s.Report.AppliedRevision, "diff": diff})
	}}
	conf.AddCommand(&cobra.Command{Use: "apply <revision>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		revision, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil || revision < 1 {
			return errors.New("invalid revision")
		}
		var out json.RawMessage
		if err := call(cmd, "apply", map[string]int64{"revision": revision}, &out); err != nil {
			return err
		}
		return printJSON(cmd, out)
	}})
	root.AddCommand(conf)
	var follow bool
	logs := &cobra.Command{Use: "logs", Short: "Show bounded recent node logs", RunE: func(cmd *cobra.Command, _ []string) error {
		var last uint64
		for {
			var lines []agent.LogEntry
			if err := call(cmd, "logs", nil, &lines); err != nil {
				return err
			}
			if len(lines) > 0 && lines[len(lines)-1].Sequence < last {
				last = 0
			}
			for _, l := range lines {
				if l.Sequence > last {
					fmt.Fprintln(cmd.OutOrStdout(), l.Line)
					last = l.Sequence
				}
			}
			if !follow {
				return nil
			}
			select {
			case <-cmd.Context().Done():
				return nil
			case <-time.After(time.Second):
			}
		}
	}}
	logs.Flags().BoolVar(&follow, "follow", false, "Follow new log entries")
	root.AddCommand(logs)
	service := &cobra.Command{Use: "service", Short: "Manage the native systemd service"}
	for _, action := range []string{"start", "stop", "restart"} {
		a := action
		service.AddCommand(&cobra.Command{Use: a, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return nodehost.Service(cmd.Context(), a) }})
	}
	root.AddCommand(service)
	root.AddCommand(&cobra.Command{Use: "update <version>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return nodehost.Update(cmd.Context(), configPath, dir, args[0])
	}})
	root.AddCommand(&cobra.Command{Use: "uninstall", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return nodehost.Uninstall(cmd.Context()) }})
	root.AddCommand(&cobra.Command{Use: "version", RunE: func(cmd *cobra.Command, _ []string) error { fmt.Fprintln(cmd.OutOrStdout(), root.Version); return nil }})
	return root
}
func printStatus(cmd *cobra.Command, s model.NodeLocalStatus) error {
	fmt.Fprintf(cmd.OutOrStdout(), "NodeLane Room %s\n控制端：%s\n登记状态：%t\n控制连接：%s\n隧道引擎：%s\n", s.Version, s.Server, s.Registered, s.Control, s.Engine)
	if s.Node != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "节点：%s (%s)\n配置：%d / %d\n公网入口：%s\n", s.Node.Name, s.Node.ID, s.Report.AppliedRevision, s.Node.Revision, s.Node.Address)
	}
	if !s.LeaseExpiresAt.IsZero() {
		fmt.Fprintln(cmd.OutOrStdout(), "证书到期：", s.LeaseExpiresAt.Local().Format(time.RFC3339))
	}
	if s.Error != "" {
		fmt.Fprintln(cmd.OutOrStdout(), "待处理：", s.Error)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "公网连通结果请查看控制台中的外部实测记录。")
	return nil
}
