# NodeLane Room V2

**NodeLane**（`nodelane.net`）旗下游戏组网子产品，版本 **0.2.0**，API **/v2**。包含单管理员 Web 管理台、PostgreSQL 共享控制状态、独立基础设施节点和 Go 客户端。数据面固定为 Nebula v1.11.1 与已授权的握手缓存补丁 `d929786cba7f`，设备身份与短期隧道证书分离。

玩家桌面客户端 `nlroom` 使用 Tauri 2 + React + TypeScript，界面位于 `desktop/`，采用独立的午夜主机主题；首次使用默认连接 `https://room.nodelane.net`。`nlroom-cli` 面向 AI、脚本和开发调试，网络后台 `nlroom-service` 独立运行。界面预览、设计令牌与桌面开发命令见 [客户端设计](docs/client.md#主机界面与设计令牌)，实际验收范围见 [验证记录](docs/validation.md)。

V2 使用全新数据库、CA 和节点/玩家身份。数据库结构版本为 3，API 仍为 /v2；仅接受空 schema 或本版本创建的当前结构，旧库不迁移、不自动补表、不清空。0.2.0 控制面与节点镜像已发布至 `docker.nodelane.net`，支持 `linux/amd64`、`linux/arm64`；摘要见 [镜像清单](deploy/IMAGES.txt)。当前检查与未验收项见 [验证记录](docs/validation.md)。

## 控制面和节点

完整的新部署步骤见 [部署指南](docs/deployment.md)。控制端同源提供随机管理入口、`/install/node.sh` 和两个架构的发布包。根路径及旧 `/admin` 返回 404，不跳转或披露管理地址。每次部署一个控制实例；多个独立实例可接入同一数据库，共享配置、CA 和会话，无需粘性会话。

1. 启动单个控制实例及 HTTPS 反代，无需预填业务环境配置。
2. 在实例终端执行 `nodelane-server admin path` 查看随机入口（Compose 加前缀 `docker compose exec control`），再执行 `nodelane-server admin bootstrap`。打开 HTTPS 域名加该入口，填写初始化码、数据库、公网地址、地址池和管理员，生成或上传 CA；配置保存到 PostgreSQL。其他独立实例选择“接入已有控制面”。入口首次生成后持久化，也可用 `--admin-path` / `NODELANE_ADMIN_PATH` 配置 `/` 加 16–128 位字母、数字、下划线或短横线，重启生效。
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

桌面玩家使用 `dist/desktop/nlroom-0.2.0-windows-amd64-setup.exe`（本地构建的未签名测试包）。在玩家账户下双击，接受 UAC 提权，完成后从开始菜单打开 **NodeLane Room**。默认连接 `https://room.nodelane.net`，填写昵称后即可读取游戏库、建房、输入邀请码加入、管理成员及诊断。正式入口经 Tauri、Named Pipe 和 Go 后台操作真实网络。

安装包使用 NSIS 3 Modern UI 2，提供 NodeLane 品牌欢迎页、安装与维护页、进度及完成页。安装入口在提权前取得玩家 SID；检查版本、架构、完整包摘要和 WireGuard 签名的 Wintun。缺少 WebView2 时校验微软签名并运行随包的官方 Evergreen 引导程序，需要联网。GUI 保持普通用户运行，后台独立运行。安装目录固定为 `%ProgramFiles%\NodeLaneRoom`，只保留程序、原生 `Uninstall.exe`、驱动、构建信息和许可；PowerShell 安装工具与 WebView2 引导程序仅在临时目录执行。

更新时退出客户端，在同一玩家账户下运行新版完整安装包。“安装与维护”显示当前和目标版本，同版本可重新安装，普通更新拒绝降级。安装器保留身份与用户绑定，停止并等待后台退出后替换程序；新版后台就绪或系统登记失败时恢复旧程序。成功后保留一份旧程序在 `%ProgramFiles%\NodeLaneRoom.previous`，不会保存第二份身份。再次运行安装包，若存在新版安装器生成的备份，可选择“恢复上一次安装的程序”；仅适用于身份格式和本机协议兼容的版本。原脚本版安装可以直接更新，脚本版备份不提供此恢复选项。

卸载在 Windows 设置的“已安装的应用”选择 **NodeLane Room → 卸载**，或双击安装目录中的 `Uninstall.exe`。独立卸载向导默认保留设备身份与用户绑定；勾选清除数据还需再次确认。卸载会停止并等待后台退出，移除程序、旧版备份、开始菜单和系统登记；停止失败时保留程序和卸载入口。共享 WebView2 不卸载。`Uninstall.exe /S` 可静默卸载并保留身份，仍需管理员授权。

当前交付为完整包手动更新，不提供在线更新源或静默自动更新。Windows 服务、UAC 和驱动的真机结果见 [验证记录](docs/validation.md)，不可将打包通过视为真机验收通过。

开发和脚本用户也可解压 Go 发布 ZIP，在玩家账户下运行 `Install.cmd`；`NodeLaneRoom.cmd` 打开已配置临时 PATH 的普通 PowerShell。该归档不含 GUI。

也可先在玩家自己的终端执行 `whoami /user` 获取 SID，再在管理员 PowerShell 手动安装：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\install.ps1 -OwnerSid 'S-1-5-21-...'
```

使用同一账户 UAC 提权安装时可省略 `-OwnerSid`。安装到 `%ProgramFiles%\NodeLaneRoom`，后台 `nlroom-service.exe` 以 SYSTEM 运行，服务注册名仍为 `NodeLaneRoom`；状态在 `%ProgramData%\NodeLaneRoom`，只有 SYSTEM/管理员能读。玩家通过授权 SID 的 Named Pipe 操作，无需继续提权。

普通终端执行（可把安装目录加入自己的 PATH）：

```powershell
$nl = "$env:ProgramFiles\NodeLaneRoom\nlroom-cli.exe"
& $nl init --server https://room.example.com --name 玩家甲
& $nl games
& $nl room create --name 周末世界 --game minecraft-java
# 将输出的 invitation.code 发给朋友；朋友在自己的设备初始化后执行：
& $nl room join <邀请码>
& $nl status --watch
& $nl room members --json
& $nl ping <成员设备ID或虚拟IP或唯一昵称>
```

加入后，已配置游戏会自动登记服务端指定的端口，用 `room members` 查看成员虚拟 IP 和开放端口。Minecraft 预置 TCP 25565，与其他游戏共用通用数据面；游戏实际监听端口须与配置一致。当前没有专用 Minecraft 公告、代理或自动 LAN 列表发现；通用单播、广播与组播发现的后续方案见 [游戏网络规划](docs/game-network.md)。

## 游戏管理

管理员打开隐藏管理入口中的“游戏管理”，粘贴 `https://store.steampowered.com/app/105600/` 这样的 Steam 商店游戏链接。服务端自动导入名称、纯文本简介并下载封面和背景图，保存为未启用草稿。填写 TCP/UDP 端口、可选结束端口和用途后启用；玩家运行 `games` 查看游戏 ID，使用 `room create --game <id>` 选择。`games --json` 包含图片相对 URL，可基于控制服务 origin 读取。

Steam 是本轮唯一自动导入来源，图像类型参见 [Steamworks 图像资源说明](https://partner.steamgames.com/doc/store/assets)。实现使用商店 `appdetails` 响应的 `header_image`、`background_raw`（缺失时使用 `background`），不需要 API Key；该商店接口不是有稳定性承诺的 Steamworks API，无法访问、字段缺失或图片损坏时导入失败，不保存半成品。图片为下载后的 JPEG/PNG，随资料存入共享 PostgreSQL，供各控制实例同源读取，不依赖玩家访问 Steam。

端口只允许 TCP/UDP 1–65535，排除诊断端口 4243；范围展开后最多 32 个协议/端口组合，不可重复。服务器在加入和心跳时登记固定端口，有效期 45 秒。配置版本冲突返回错误，须刷新后重新编辑；更改会同步替换已有房间端口，客户端按原有 Stop/Wait 机制重启数据面。停用禁止新建和加入该游戏房间，并收回现有游戏端口。

未收录游戏可选“通用游戏”（`custom`，默认类型、始终可用），每位玩家自行登记本机需要的端口：

```powershell
& $nl room create --name 自定义联机 --game custom
& $nl room port tcp/7777
& $nl room port udp/27015
& $nl room port udp/27015 --remove
```

通用类型同样最多 32 个端口，仅允许操作本机登记，不能修改其他成员。删除停止续登并通知服务端立即移除；请求失败会报错，残留授权最多等原 45 秒租期结束。离房清除本机端口设置。已配置游戏禁止客户端追加或删除服务器指定端口。

## 操作

| 命令 | 行为 |
|---|---|
| `room invite` | 生成有效 30 分钟的新邀请码，旧码立即失效；可多人使用至房满 |
| `room kick <device-id>` | 踢出并禁止该设备再次用邀请码进入本房；撤销全部旧证书 |
| `room transfer <device-id>` | 转让房间管理权，不迁移游戏进程或存档 |
| `room leave` | 退出房间、停止本机网络；房主保留房间管理权 |
| `room close` | 房主关闭房间，撤销所有成员凭据 |
| `room … --room <id>` | 显式选择管理的房间；房主离房后管理时使用 |
| `games`、`games --json` | 读取服务端已启用游戏、配置端口和资料 |
| `room port tcp/25565`、`room port udp/27015 --remove` | 通用游戏增加或删除本机端口；持续登记到删除或离房为止 |
| `status --watch`、`peers`、`ping <成员>` | 分别显示控制状态、Nebula 路径与真实探测结果；支持 `--json` |
| `doctor` | 查看 Wintun 文件、网卡、凭据、控制状态和探测结果 |
| `nlroom-service service install/uninstall` | 注册或删除 Windows 服务，需要管理员 |

每设备同时一房，每房最多 32 人，有效期 24 小时；房主离线不关闭房间。玩家设备身份绑定本机私钥；管理台使用独立的管理员账号密码。默认地址池 `10.203.0.0/16`，页面初始化时可改，运行中不能直接换池。

证书最多 10 分钟，剩余约 7 分钟开始续签。控制失联期间不接受新操作，已有链路最多保留至当前凭据到期；实际可用时间也取决于对端和 relay 的剩余凭据。端口权限变化会受控重启 Nebula，短暂重连，这是规避固定上游版本防火墙热更新竞争的措施。

仅 Go 开发归档使用发布包中的 `uninstall.ps1`：可先 `room leave`，再在管理员 PowerShell 执行卸载脚本。默认保留设备身份；`-PurgeState` 同时清除身份。后台退出会关闭隧道和 WFP 动态会话，并释放 Nebula TUN。

## Linux 桌面与客户端开发

桌面使用 `dist/desktop/nlroom_0.2.0_amd64.deb`，在 Ubuntu 22.04/24.04、Debian 12/13 上由 apt 安装依赖；首次安装需显式绑定玩家：

```sh
sudo apt install ./nlroom_0.2.0_amd64.deb
sudo nlroom-setup --owner "$USER"
nlroom
```

后续退出 GUI，再用相同 apt 命令安装新版 deb。升级停止旧后台，保留 UID 和身份，并重启已启用的后台；就绪失败会让软件包配置报错。修复后执行 `sudo dpkg --configure nlroom`，或使用兼容的旧 deb 显式降级。系统管理员禁用的服务不会自动启用。`sudo apt remove nlroom` 卸载程序，保留 `/var/lib/nlroom` 与 `/etc/nlroom`；purge 同样保留设备身份，销毁身份须自行明确处理这两个目录。

Linux Go 归档包含 `nlroom-cli` 和 `nlroom-service`，用于开发和隔离回归。显式 `--state-dir` 下 socket 为 `0600`，CLI 和后台必须使用同一系统用户。桌面版另提供 root 服务与绑定安装用户 UID 的独立 socket；安装包构建和用户绑定见 [客户端设计](docs/client.md#桌面架构)，宿主安装尚待真机验收。

在具有 TUN 权限的隔离开发环境，两个终端使用相同用户执行：

```sh
# 终端一，前台运行后台服务
nlroom-service --state-dir /state/client daemon
# 终端二
nlroom-cli --state-dir /state/client init --server https://room.example.com --name 玩家甲
nlroom-cli --state-dir /state/client status --json
```

## 开发与构建

管理台使用 React 19、TypeScript 和 Vite，Node.js 24 LTS 负责开发与构建，Go 同源提供编译后的页面与 API，生产环境无需运行 Node.js。前端位于 `internal/control/adminweb/`，依赖由 `package-lock.json` 锁定。Go 最低版本由 `go.mod` 声明，CI/容器使用 1.26.8。开发约束见 [AGENTS.md](AGENTS.md)。

```sh
npm --prefix internal/control/adminweb ci
npm --prefix internal/control/adminweb run build
npm --prefix internal/control/adminweb test
go mod verify
go vet ./...
go test -count=1 ./...
# Linux，数据库用户须能创建测试 schema；每个用例自动隔离和清理 schema。
NODELANE_TEST_DATABASE_URL='postgres://user:password@localhost/nodelane_test?sslmode=disable' go test -race -count=1 ./...
bash scripts/build.sh
```

Go 包边界与依赖检查见 [架构说明](docs/architecture.md#代码组织)。

Go 编译前须构建前端，`go:embed` 嵌入 `internal/control/adminweb/dist/`；构建目录与 `node_modules` 不入库。发布脚本、源码 Dockerfile 与 CI 已自动执行此步骤。`npm --prefix internal/control/adminweb run dev` 启动 Vite；完整登录与初始化验收使用 Go 提供的随机管理入口，以满足已配置的同源 Origin。

Windows 上构建 Windows/Linux amd64、arm64 归档（PowerShell 5.1+）：

```powershell
.\scripts\build.ps1 -Go 'C:\Program Files\Go\bin\go.exe'
# 只更新服务端归档时：
.\scripts\build.ps1 -Targets 'linux/amd64','linux/arm64'
```

产物默认在 `dist/0.2.0/`，包含 Windows ZIP 安装包、Linux tar.gz、SHA256SUMS、Wintun 和第三方许可。Windows 包包含 `nlroom-cli.exe`、`nlroom-service.exe` 及安装入口；Linux 包包含相同两项客户端程序、控制面和节点二进制、运行镜像 Dockerfile、控制面与数据节点的 Compose 编排，解压后无需源码或 Go 即可构建部署。发布包不包含 README、AGENTS、docs 或驱动使用说明；操作步骤见源码中的 [部署指南](docs/deployment.md)，法律声明和许可证保留。可执行文件尚未由 NodeLane 代码签名证书签名。构建脚本不安装服务、不创建云资源。

构建后用 `python scripts/check-release.py dist/0.2.0` 检查归档的 SHA256、内容、架构和 Linux 执行权限。

桌面完整包另行构建，需要 Rust 1.95、Node.js 24、Windows NSIS 或 Linux WebKitGTK 4.1 系统依赖。GUI、Rust、Go 和传入发布目录的版本必须一致；构建器校验三个程序的架构并记录 GUI 摘要，产物附带 SHA256。在对应平台执行：

```sh
python scripts/desktop/build.py --platform windows --arch amd64 --release dist/0.2.0/nodelane-room-0.2.0-windows-amd64
python scripts/desktop/build.py --platform linux --arch amd64 --release dist/0.2.0/nodelane-room-0.2.0-linux-amd64
```

`scripts/desktop/Dockerfile` 提供 Ubuntu 22.04 构建环境、Windows 交叉编译工具和原生 WebView 验收工具。`--skip-web` 仅复用已构建并检查的前端资源。当前生成的 EXE 为未签名测试包，deb 尚未进入签名软件仓库；构建不安装宿主服务、不发布产物。ARM64 参数用于相应工具链，未通过 ARM 真机验收。

桌面检查：`npm --prefix desktop test`、`cargo test --manifest-path desktop/src-tauri/Cargo.toml --locked`、`python scripts/desktop/test_package.py`，Windows 另运行 `scripts/desktop/test-windows.ps1`。打包后可构建上述 Dockerfile 为 `nodelane-desktop-build:local`，再运行 `python scripts/desktop/test_live.py --package dist/desktop/nlroom_0.2.0_amd64.deb`，通过真实 WebView、普通用户 socket、隔离 HTTPS/数据库与 Nebula 验证客户端流程。它仅安装到临时容器，结束后清理；不代替宿主 systemd 或 Windows 服务验收。

0.2.0 发布镜像已包含页面初始化和游戏目录功能。控制面选择全套或已有基础设施两种独立模板，节点使用单独的 compose.node.yaml。完整步骤和必填项见 [部署指南](docs/deployment.md)。

1Panel 使用已有 `1panel-network` 时，可将 `deploy/compose.network.yaml` 的 `networks` 段放入精简编排顶层，或在 CLI 用第二个 `-f` 叠加该文件；控制服务加入此网络。PostgreSQL 在同一网络时，连接串可使用其实际容器名与内部端口。游戏地址池在页面填写，与 Docker 网络分开。详见 [1Panel 网络配置](docs/deployment.md)。

## 实时网络监控

管理台展示节点、房间和成员的实际链路、RTT/丢包、流量趋势及观察到的出口和地区；无有效观测时显示未知。采样与统计口径见 [架构说明](docs/architecture.md#监控)，Linux 采集权限、GeoIP 自动下载及本地库配置见 [部署指南](docs/deployment.md#监控与-ip-归属地)。

## Docker 双客户端回归

在源码根目录运行 `python scripts/test-docker.py`。需要 Python 3.10+、Docker Compose 和提供 `/dev/net/tun` 的 Linux Docker 引擎（包括满足条件的 Docker Desktop）。首次运行需要下载镜像与 Go 依赖，不使用 `dist` 产物。加 `--verify` 会额外在 Linux 容器中使用独立 PostgreSQL 测试库执行 `go vet ./...`、`go test -count=1 ./...`、`go test -race -count=1 ./...`；race 失败会保留失败状态和日志。

两个玩家容器分别连接隔离的 `172.30.81.0/24`、`172.30.82.0/24` 网络；容器内还配置对侧网段不可达路由。额外三个容器提供测试控制面、PostgreSQL、固定补丁版本的 Nebula lighthouse/relay。基础设施关闭 IP 转发，不发布宿主端口，脚本不调用宿主防火墙或路由配置命令。测试网段须与现有 Docker 网络不重叠；如果 Docker 报冲突，在 `deploy/test/compose.yaml` 和测试脚本中同步选择空闲网段。

脚本先证实底层 TCP/UDP 双向不通，再测试注册、邀请换新、建房/入房、真实 TUN 中继通信、端口默认拒绝/放行、跨房拒绝、离房/重入/关房，游戏目录、固定端口自动登记、配置在线替换、自定义端口删除，以及踢人撤销。测试使用真实 TCP/UDP 回显服务，不运行游戏本体；此拓扑没有模拟运营商 NAT，也不覆盖 Windows 驱动/服务验收。

每次运行创建唯一 Compose 项目。临时密钥、身份和数据库保存在容器临时存储中，只有 HTTPS 公钥证书共享给客户端；邀请码和登记令牌不写入日志。退出时清理该次容器、网络、证书卷和带本次唯一标签的测试镜像，检查结果与构建/验证日志保存在 `.local/nodelane-test-*/`。源码配置位于 `deploy/test/`，不会放入发布包。

部署模板冒烟使用 `python scripts/test-deploy.py`；加 `--host` 测试复用现有设施的精简编排。可加 `--root dist/0.2.0/nodelane-room-0.2.0-linux-amd64` 验证发布包，或 `--images --pull` 验证仓库发布镜像。`--extended` 额外验证真实十分钟证书续签与断控到期。它测试单实例页面初始化、数据库与 CA 保存、Caddy 内部测试 HTTPS、令牌签发、节点登记、真实 TUN、持久化身份和控制容器重建恢复，不开放宿主端口；不代替宿主网关/端口连通性、现有反代配置、公网证书和 Windows 真机验收。

源码入口见 [文件索引](docs/files.md)，调用方与权限分工见 [架构说明](docs/architecture.md)，接口见 [OpenAPI](docs/openapi.yaml)。V2 不包含多管理员角色、TOTP、云资源自动创建或自动 LAN 发现（通用方案见游戏网络规划）。
