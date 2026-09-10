package agent

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/nodelane/nodelane-room/internal/engine"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
)

type DoctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Advice string `json:"advice,omitempty"`
}

func (r *Runtime) nodeDoctor(ctx context.Context) any {
	status := r.NodeStatus()
	checks := []DoctorCheck{}
	add := func(name string, err error, advice string) {
		c := DoctorCheck{Name: name, OK: err == nil}
		if err != nil {
			c.Detail = err.Error()
			c.Advice = advice
		}
		checks = append(checks, c)
	}
	if runtime.GOOS == "linux" {
		f, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
		if f != nil {
			f.Close()
		}
		add("TUN device access", err, "宿主机启用 TUN，并允许服务账号读写；容器挂载 /dev/net/tun。")
		b, err := os.ReadFile("/proc/self/status")
		if err == nil {
			err = fmt.Errorf("CAP_NET_ADMIN is missing")
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(line, "CapEff:") {
					v, e := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "CapEff:")), 16, 64)
					if e == nil && v&(1<<12) != 0 {
						err = nil
					}
					break
				}
			}
		}
		add("CAP_NET_ADMIN", err, "原生部署检查 systemd AmbientCapabilities；容器使用 cap_add: [NET_ADMIN]。")
	}
	network := model.DefaultPool
	r.netMu.Lock()
	if r.lease.Network != "" {
		network = r.lease.Network
	}
	r.netMu.Unlock()
	add("Overlay address conflict", engine.CheckAddressConflict(network, "nodelane0"), "调整与组网地址池重叠的本机 LAN、Docker 或其他 VPN 网络；不要删除未知路由。")
	u, err := url.Parse(status.Server)
	if err == nil {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		ips, e := net.DefaultResolver.LookupIPAddr(c, u.Hostname())
		cancel()
		add("Control DNS", e, "检查服务器 DNS 解析和控制域名。")
		if e == nil {
			checks[len(checks)-1].Detail = fmt.Sprint(ips)
		}
		c, cancel = context.WithTimeout(ctx, 8*time.Second)
		req, e := http.NewRequestWithContext(c, "GET", status.Server+"/readyz", nil)
		if e == nil {
			h := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			response, requestErr := h.Do(req)
			e = requestErr
			if response != nil {
				response.Body.Close()
				if e == nil && response.StatusCode != 200 {
					e = fmt.Errorf("control readiness HTTP %d", response.StatusCode)
				}
				if serverTime, te := http.ParseTime(response.Header.Get("Date")); te == nil {
					delta := time.Since(serverTime)
					var clockErr error
					if delta > 30*time.Second || delta < -30*time.Second {
						clockErr = fmt.Errorf("clock difference %s", delta.Round(time.Second))
					}
					add("Clock", clockErr, "启用主机时间同步后重试。")
				}
			}
		}
		cancel()
		add("HTTPS and control readiness", e, "检查 HTTPS 信任、出口 443、反代、控制服务和数据库。")
	}
	if status.Node != nil && os.Getenv("NLROOM_DEPLOYMENT") == "container" {
		_, p, e := net.SplitHostPort(status.Node.Address)
		if e == nil && os.Getenv("NLROOM_MAPPED_PORT") != p {
			e = fmt.Errorf("desired UDP %s differs from declared mapping %s", p, os.Getenv("NLROOM_MAPPED_PORT"))
		}
		add("Declared container UDP mapping", e, "更新 YAML 的 UDP 端口映射和 NLROOM_MAPPED_PORT，重建后执行 config apply。映射实际可达性仍需外部探测。")
	}
	return map[string]any{"environment": platform.Diagnostics(), "node": status, "checks": checks, "external_connectivity": "请查看控制台的外部实测来源、时间和路径；HTTPS 成功不能证明 UDP 直达。"}
}
