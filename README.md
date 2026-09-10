# NodeLane Room V2

Go 游戏组网产品，版本 **0.2.0**，API **/v2**。包含单管理员 Web 管理台、PostgreSQL 双控制副本、独立基础设施节点和 Windows CLI/服务。数据面固定为 Nebula v1.11.1 与已授权的握手缓存补丁 `d929786cba7f`，设备身份与短期隧道证书分离。

V2 使用全新数据库、CA 和节点/玩家身份。初始化仅接受空数据库 schema 或 V2 schema，遇到 V1/其他版本退出，不清库、不自动转换。0.2.0 控制面与节点镜像已发布至 `docker.nodelane.net`，支持 `linux/amd64`、`linux/arm64`；摘要见 [镜像清单](deploy/IMAGES.txt)。当前检查与未验收项见 [验证记录](docs/validation.md)。

## 控制面和节点

完整的新部署步骤见 [部署指南](docs/deployment.md)。控制端同源提供 `/admin`、`/install/node.sh` 和两个架构的发布包。控制副本共用数据库及 CA，无需粘性会话。

1. 初始化空数据库和新 CA，启动两个控制副本及 HTTPS 反代。
2. 在控制服务器执行 `nodelane-server admin bootstrap`，取得 10 分钟一次性初始化码，在 `/admin` 设置管理员账号密码。
3. 管理台创建节点，填写公网域名/IP、UDP 端口、区域和角色；生成 30 分钟临时接入密钥。密钥仅显示一次。
4. 在节点服务器选择一种部署方式，登记成功后自动启动数据面。

原生安装只操作当前宿主机，适用于 Debian 12/13、Ubuntu 22.04/24.04 的 systemd 主机：

```bash
curl -fsSL https://room.example.com/install/node.sh | bash -s -- --server https://room.example.com
# 按终端提示隐藏输入密钥，随后：
nlroom-node status
nlroom-node doctor
```

脚本不会根据 Docker 的存在切换安装目标，在容器内执行会拒绝。容器独立使用 `deploy/compose.node.yaml` 或管理台生成的 YAML；镜像内置同一个 `nlroom-node`：

```bash
docker compose -f compose.node.yaml up -d --wait
docker compose -f compose.node.yaml exec node nlroom-node enroll
```

首次容器启动会保持等待登记；`/healthz` 反映进程存活，`/readyz` 才表示数据面就绪。接入密钥不写入 YAML、环境变量或命令行参数。日常认证、心跳和证书续签由节点自动处理。

## Windows 客户端

Windows 解压 `dist/0.2.0/` 中对应架构的 ZIP（Intel/AMD 电脑选 `windows-amd64`，Windows ARM 电脑选 `windows-arm64`），在玩家账户下双击 `Install.cmd`，接受 UAC 提权。安装入口会在提权前取得玩家 SID；安装脚本检查架构、必需文件和 WireGuard 签名的 Wintun，并等待服务及本地管道就绪。安装后双击 `NodeLane.cmd` 打开已配置临时 PATH 的普通 PowerShell。

也可先在玩家自己的终端执行 `whoami /user` 获取 SID，再在管理员 PowerShell 手动安装：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\install.ps1 -OwnerSid 'S-1-5-21-...'
```

使用同一账户 UAC 提权安装时可省略 `-OwnerSid`。安装到 `%ProgramFiles%\NodeLaneRoom`，后台以 SYSTEM 运行；状态在 `%ProgramData%\NodeLaneRoom`，只有 SYSTEM/管理员能读。玩家通过授权 SID 的 Named Pipe 操作，无需继续提权。

普通终端执行（可把安装目录加入自己的 PATH）：

```powershell
$nl = "$env:ProgramFiles\NodeLaneRoom\nodelane.exe"
& $nl init --server https://room.example.com --name 玩家甲
& $nl room create --name 周末世界 --game minecraft-java
# 将输出的 invitation.code 发给朋友；朋友在自己的设备初始化后执行：
& $nl room join <邀请码>
& $nl status --watch
& $nl room members --json
& $nl ping <成员设备ID或虚拟IP或唯一昵称>
```

房主启动**原版 Minecraft Java 1.21.1**，进入世界并选择“开放到局域网”。远端玩家打开多人游戏列表，等待约 5–15 秒。适配器验证本机监听端口后登记端点，经 Nebula 单播通知房内成员，在接收机器发布本机代理端口。列表不可见时，可使用 `room members` 的虚拟 IP 和房主 LAN 端口直接加入。见 [游戏与网络验收](docs/validation.md)。

## 操作

| 命令 | 行为 |
|---|---|
| `room invite` | 生成有效 30 分钟的新邀请码，旧码立即失效；可多人使用至房满 |
| `room kick <device-id>` | 踢出并禁止该设备再次用邀请码进入本房；撤销全部旧证书 |
| `room transfer <device-id>` | 转让管理权；Minecraft 世界仍在原游戏主机 |
| `room leave` | 退出房间、停止本机网络；房主保留房间管理权 |
| `room close` | 房主关闭房间，撤销所有成员凭据 |
| `room … --room <id>` | 显式选择管理的房间；房主离房后管理时使用 |
| `room port tcp/25565`、`room port udp/27015` | 增加本机游戏端口，持续登记到离房为止；其他游戏建 `--game custom` 房间 |
| `status --watch`、`peers`、`ping <成员>` | 分别显示控制状态、Nebula 路径与真实探测结果；支持 `--json` |
| `doctor` | 查看 Wintun 文件、网卡、凭据、控制状态和探测结果 |
| `service install/uninstall` | 注册或删除 Windows 服务，安装需要管理员 |

每设备同时一房，每房最多 32 人，有效期 24 小时；房主离线不关闭房间。玩家设备身份绑定本机私钥；管理台使用独立的管理员账号密码。默认地址池 `10.203.0.0/16`，部署前可改，运行中不能直接换池。

证书最多 10 分钟，剩余约 7 分钟开始续签。控制失联期间不接受新操作，已有链路最多保留至当前凭据到期；实际可用时间也取决于对端和 relay 的剩余凭据。端口权限变化会受控重启 Nebula，短暂重连，这是规避固定上游版本防火墙热更新竞争的措施。

卸载前先 `room leave`，再在管理员 PowerShell 执行发布包中的 `uninstall.ps1`。默认保留设备身份；`-PurgeState` 同时清除身份。升级前卸载服务并保留身份，再装新包。后台退出会关闭隧道、本机代理和 WFP 动态会话，并释放 Nebula TUN。

## 开发与构建

Go 最低版本由 `go.mod` 声明，CI/容器使用 1.26.8。依赖锁定在 `go.mod`/`go.sum`。开发约束见 [AGENTS.md](AGENTS.md)。

```sh
go mod verify
go vet ./...
go test -count=1 ./...
# Linux，数据库用户须能创建测试 schema；每个用例自动隔离和清理 schema。
NODELANE_TEST_DATABASE_URL='postgres://user:password@localhost/nodelane_test?sslmode=disable' go test -race -count=1 ./...
bash scripts/build.sh
```

Windows 上构建 Windows/Linux amd64、arm64 归档（PowerShell 5.1+）：

```powershell
.\scripts\build.ps1 -Go 'C:\Program Files\Go\bin\go.exe'
# 只更新服务端归档时：
.\scripts\build.ps1 -Targets 'linux/amd64','linux/arm64'
```

产物在 `dist/0.2.0/`，包含 Windows ZIP 安装包、Linux tar.gz、SHA256SUMS、Wintun 和第三方许可。Windows 包只包含客户端及安装入口；Linux 包带控制面和节点二进制、运行镜像 Dockerfile、控制面与数据节点的 Compose 编排，解压后无需源码或 Go 即可构建部署。发布包不包含 README、AGENTS、docs 或驱动使用说明；操作步骤见源码中的 [部署指南](docs/deployment.md)，法律声明和许可证保留。可执行文件尚未由 NodeLane 代码签名证书签名。构建脚本不安装服务、不创建云资源。

构建后用 `python scripts/check-release.py dist/0.2.0` 检查归档的 SHA256、内容、架构和 Linux 执行权限。

容器部署可直接拉取 `docker.nodelane.net` 的 0.2.0 版本镜像。控制面选择全套或已有基础设施两种独立模板，节点使用单独的 compose.node.yaml。完整步骤和必填项见 [部署指南](docs/deployment.md)。

1Panel 使用已有 `1panel-network` 时，可将 `deploy/compose.network.yaml` 的 `networks` 段放入精简编排顶层，或在 CLI 用第二个 `-f` 叠加该文件；`migrate` 和控制服务均加入此网络。PostgreSQL 在同一网络时，连接串可使用其实际容器名与内部端口。`NODELANE_NETWORK` 仍表示游戏地址池。详见 [1Panel 网络配置](docs/deployment.md)。

## Docker 双客户端回归

在源码根目录运行 `python scripts/test-docker.py`。需要 Python 3.10+、Docker Compose 和提供 `/dev/net/tun` 的 Linux Docker 引擎（包括满足条件的 Docker Desktop）。首次运行需要下载镜像与 Go 依赖，不使用 `dist` 产物。加 `--verify` 会额外在 Linux 容器中使用独立 PostgreSQL 测试库执行 `go vet ./...`、`go test -count=1 ./...`、`go test -race -count=1 ./...`；race 失败会保留失败状态和日志。

两个玩家容器分别连接隔离的 `172.30.81.0/24`、`172.30.82.0/24` 网络；容器内还配置对侧网段不可达路由。额外三个容器提供测试控制面、PostgreSQL、固定补丁版本的 Nebula lighthouse/relay。基础设施关闭 IP 转发，不发布宿主端口，脚本不调用宿主防火墙或路由配置命令。测试网段须与现有 Docker 网络不重叠；如果 Docker 报冲突，在 `deploy/test/compose.yaml` 和测试脚本中同步选择空闲网段。

脚本先证实底层 TCP/UDP 双向不通，再测试注册、邀请换新、建房/入房、真实 TUN 中继通信、端口默认拒绝/放行、跨房拒绝、离房/重入/关房，以及模拟 Minecraft 公告、TTL 0 本机发现、真实 TCP 代理和踢人断开已有代理连接。Minecraft 测试使用协议模拟器，不运行游戏本体；此拓扑没有模拟运营商 NAT，也不覆盖 Windows 驱动/服务验收。

每次运行创建唯一 Compose 项目。临时密钥、身份和数据库保存在容器临时存储中，只有 HTTPS 公钥证书共享给客户端；邀请码和登记令牌不写入日志。退出时清理该次容器、网络、证书卷和带本次唯一标签的测试镜像，检查结果与构建/验证日志保存在 `.local/nodelane-test-*/`。源码配置位于 `deploy/test/`，不会放入发布包。

部署模板冒烟使用 `python scripts/test-deploy.py`；加 `--host` 测试复用现有设施的精简编排。可加 `--root dist/0.2.0/nodelane-room-0.2.0-linux-amd64` 验证发布包，或 `--images --pull` 验证仓库发布镜像。它测试 CA 初始化、自动迁移、双副本、Caddy 内部测试 HTTPS、令牌签发、节点登记、真实 TUN、持久化身份和副本停止，不开放宿主端口；不代替宿主网关/端口连通性、现有反代配置、公网证书和 Windows 真机验收。

源码入口见 [文件索引](docs/files.md)，调用方与权限分工见 [架构说明](docs/architecture.md)，接口见 [OpenAPI](docs/openapi.yaml)。V2 不包含玩家 GUI、多管理员角色、TOTP、云资源自动创建或其他游戏的自动发现。
