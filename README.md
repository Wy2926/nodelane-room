# NodeLane Room V2

**NodeLane**（`nodelane.net`）旗下游戏组网子产品，控制面 **0.3.1**、节点 **0.2.1**、客户端 **0.3.0**，API **/v2**。包含公开官网、单管理员 Web 管理台、PostgreSQL 共享控制状态、独立基础设施节点和 Go 客户端。数据面固定为 Nebula v1.11.1 与已授权的握手缓存补丁 `d929786cba7f`，设备身份与短期隧道证书分离。

玩家桌面客户端 `nlroom` 使用 Tauri 2 + React + TypeScript，首次使用默认连接 `https://room.nodelane.net`。`nlroom-cli` 面向脚本和开发调试，网络后台 `nlroom-service` 独立运行。界面设计见 [客户端设计](docs/client.md)，修改代码先用 [任务导航](docs/files.md#按任务读取) 定位。

V2 使用全新数据库、CA 和节点/玩家身份。数据库结构版本为 6，API 仍为 /v2；仅接受空 schema 或本版本创建的当前结构，旧库不迁移、不自动补表、不清空。0.2.1 包含当前 Ethernet LAN 架构；[镜像清单](deploy/IMAGES.txt) 提供镜像摘要，历史 0.2.0 使用旧数据面，不能与当前客户端混用。本机 IPC 为版本 3，HTTP 与 IPC 统一使用 `interaction-1` 契约，不兼容旧客户端或旧响应。当前检查与未验收项见 [验证记录](docs/validation.md)。

## 控制面和节点

部署步骤单处维护于 [部署指南](docs/deployment.md)：启动控制实例及 HTTPS 反代 → 用实例初始化码配置数据库与 CA → 管理台创建节点并签发临时接入密钥 → 选择 [原生安装](docs/deployment.md#路径一curl-原生安装) 或 [Compose](docs/deployment.md#路径二compose--1panel) 完成登记。密钥不写入 YAML、环境变量或命令行参数。

域名根路径默认打开中文官网，首页、产品、下载、帮助、关于、隐私和使用说明共 7 类页面均提供中英版本，共 14 个入口；语言切换保留当前页面。官网沿用桌面应用的暖白与珊瑚色风格并展示对应语言的应用截图，由 Go 随程序提供，初始化前也可访问。管理入口继续使用随机私有路径，`/admin` 返回 404；入口查询、管理操作划分与部署维护见 [部署指南](docs/deployment.md#官网与管理入口)。多实例接入同一 PostgreSQL 共享控制状态。

官网下载页读取管理台发布的可用客户端版本，按系统和架构选择安装包；发布、存储源和推荐版本的对应关系见 [官网下载](docs/updates.md#官网下载)。客户端更新的签名发布、多源配置、强制规则和恢复流程见同一文档。控制面 0.3.1 与节点 0.2.1 双架构镜像已发布，摘要见 [镜像清单](deploy/IMAGES.txt)。Windows/Linux amd64 客户端 0.3.0 完整包已通过 TUF 签名发布至官网下载页；Windows 包尚未配置 Authenticode 发布者签名。

## Windows 客户端

桌面玩家使用 `dist/desktop/nlroom-0.3.0-windows-amd64-setup.exe`（按当前源码构建；正式发布前完成代码签名）。在玩家账户下双击，先选择简体中文或 English，接受 UAC 提权，完成后从开始菜单打开 **NodeLane Room**。首次打开客户端先选择语言，后续自动记住，可在设置 → 桌面偏好中更改。GUI 使用 `https://room.nodelane.net`，不提供服务端地址设置；登录账号或填写访客昵称后即可建房、输入邀请码加入和管理成员；进入网络诊断页自动检查。正式入口经 Tauri、Named Pipe 和 Go 后台操作真实网络。

安装包使用 NSIS 3 Modern UI 2，由原生 `nlroom-update.exe` 完成安装、升级与卸载，不执行 PowerShell。助手在提权前取得玩家 SID，检查版本、架构、完整包摘要与权限，并通过受限 Named Pipe 回传结果。安装目录固定为 `%ProgramFiles%\NodeLaneRoom`，包含程序、`Uninstall.exe`、构建信息、许可和供启动时使用的 `drivers/tap/`。GUI 保持普通用户运行，后台独立运行。WebView2 作为现有系统环境前提，安装器不检测、下载或安装运行时。

已安装的 GUI 每次启动时由原生助手检查 `nodelane0-lan`，通过 Windows PnP 获取实际设备的驱动登记，排除已移除设备的连接残留。已有合格网卡直接继续；缺失时请求 UAC，核对原安装玩家 SID，再校验随包驱动并创建专用网卡。取消授权或安装失败时仍可查看设置和诊断，处理问题后重启客户端重试。源码目录中的开发 GUI 不自动安装宿主驱动。

更新时在同一玩家账户下运行新版完整安装包。“安装与维护”显示当前和目标版本，同版本可重新安装，普通更新拒绝降级。安装器保留身份与用户绑定，关闭客户端、停止并等待后台退出后覆盖程序，完成后检查新版后台。失败或中断后重新运行同一版本或新版完整安装包修复；不自动回滚、不保留旧版备份、不创建开机恢复任务。

卸载在 Windows 设置的“已安装的应用”选择 **NodeLane Room → 卸载**，或双击安装目录中的 `Uninstall.exe`。独立卸载向导默认保留设备身份与用户绑定；勾选清除数据还需再次确认。卸载先取得与更新共用的锁，再停止并等待后台退出，移除程序、原生助手创建并登记的专用网卡、旧版遗留备份、开始菜单和系统登记；停止失败时保留程序和卸载入口。共享 TAP 驱动包保留。清除身份后只保留空的受保护状态目录及锁文件。`Uninstall.exe /S` 可静默卸载并保留身份，仍需管理员授权。

设置 → 版本与更新提供签名包检查、下载与安装入口；发布、可信根和服务端配置见 [客户端更新](docs/updates.md)。Windows 在线更新与手动完整包使用同一个原生安装流程，常规升级不安装驱动。Windows 服务、UAC 和驱动的真机结果见 [验证记录](docs/validation.md)，不可将打包通过视为真机验收通过。

开发和脚本用户也可解压 Go 发布 ZIP，在玩家账户下运行 `Install.cmd`；`NodeLaneRoom.cmd` 打开已配置临时 PATH 的普通 PowerShell。该归档不含 GUI。

也可先在玩家自己的终端执行 `whoami /user` 获取 SID，再在管理员 PowerShell 手动安装：

```powershell
.\nlroom-update.exe setup --source . --owner-sid 'S-1-5-21-...'
```

使用同一账户 UAC 提权安装时可省略 `--owner-sid`。安装到 `%ProgramFiles%\NodeLaneRoom`，后台 `nlroom-service.exe` 以 SYSTEM 运行，服务注册名仍为 `NodeLaneRoom`；状态在 `%ProgramData%\NodeLaneRoom`，只有 SYSTEM/管理员能读。玩家通过授权 SID 的 Named Pipe 操作，无需继续提权。

普通终端执行（可把安装目录加入自己的 PATH）：

```powershell
$nl = "$env:ProgramFiles\NodeLaneRoom\nlroom-cli.exe"
& $nl init --server https://room.example.com --name 玩家甲
& $nl games
& $nl room create --game-revision <游戏当前revision> --name 周末世界 --game minecraft-java
# 将输出的 invitation.code 发给朋友；朋友在自己的设备初始化后执行：
& $nl room join <邀请码>
& $nl status --watch
& $nl room members --json
& $nl ping <成员设备ID或虚拟IP或唯一昵称>
```

加入后，已配置游戏自动应用服务端指定的端口，用 `room members` 查看成员虚拟 IP 和规则。Minecraft 预置 TCP 25565，与其他游戏共用通用数据面；游戏实际监听端口须与配置一致。所有游戏统一使用 Ethernet LAN，管理员配置房内广播、组播及直接连接权限；客户端准备与使用见下文，技术设计及兼容边界见 [游戏网络](docs/game-network.md)。

账号的“限制创建房间”仅禁止新建，仍可登录、加入和管理已有房间；GUI 显示具体限制与刷新入口。需要撤销连接时使用管理台的设备撤销或账号删除，详见[用户身份](docs/architecture.md#用户身份)。

## 游戏管理

管理员打开隐藏管理入口中的“游戏管理”，粘贴 `https://store.steampowered.com/app/105600/` 这样的 Steam 商店游戏链接。服务端自动导入名称、纯文本简介并下载封面和背景图，保存为未启用草稿。填写 TCP/UDP 端口、可选结束端口和用途后启用；玩家运行 `games` 查看游戏 ID，使用 `room create --game <id>` 选择。`games --json` 包含图片相对 URL，可基于控制服务 origin 读取。

Steam 是当前唯一自动导入来源，无需 API Key；外部商店接口没有稳定性承诺，无法访问、字段缺失或图片损坏时导入失败，不保存半成品。图片随资料保存到 PostgreSQL，玩家无需访问 Steam。

端口允许 TCP/UDP 1–65535，用起止区间填写，规则及覆盖数量不设上限，区间不可重叠。诊断不占用游戏 UDP 4243。配置版本冲突须刷新后重新编辑；规则变化会使已有房间短暂重连。停用禁止新建/加入并收回现有游戏流量授权；授权机制见 [游戏网络](docs/game-network.md#授权与交付)。

未收录游戏可选“通用游戏”（`custom`），默认开放完整 TCP/UDP 范围与广播/组播，管理员可修改或停用。客户端只选择游戏，不编辑网络权限，无 `room port` 或端口登记操作。

## Ethernet LAN 客户端准备

管理台可配置广播、组播；额外 Ethernet 类型仅在老游戏需要时填写，如 IPX 的 `0x8137`、IEEE 802.3 的 `0`。动态端口游戏可配置 TCP、UDP 各一条 `1–65535`。须使用支持当前 LAN 与 IPC 的匹配客户端，能力字段见游戏网络文档。

- Windows：完整桌面包内置 [TAP-Windows6 9.27.0](https://github.com/OpenVPN/tap-windows6/releases/tag/9.27.0) 与 OpenVPN 2.6.22 的 `tapctl`，固定下载摘要并通过 Windows Authenticode 核对 Microsoft/OpenVPN 签名。`tapctl` 仅用于管理网卡；`licenses/tap-windows6/` 中的两个 `.tar.gz` 是随附源码及许可，不执行，也不安装 OpenVPN 客户端或服务。GUI 启动时仅在缺少专用网卡时提权暂存驱动、创建 **`nodelane0-lan`**，MTU 为 1500；不更新或重命名其他 VPN 网卡。已有同名网卡须为官方 TAP ID（`root\tap0901` 或 `tap0901`）且驱动至少 9.27.0，否则报告错误。只有本次新建的 GUID 写入受保护的 `tap.guid`，失败时清理、卸载时核对名称和 GUID 后移除；手动准备的网卡保留。CLI 开发归档仍需管理员自行准备 TAP。网络服务只打开网卡并配置、清理地址，不执行驱动安装。Windows 驱动实际加载及完整安装卸载仍须真机验收。
- Linux：后台以 root 或所需网络管理权限运行，使用内核 `/dev/net/tun` 创建非持久 `nodelane0-lan` TAP，停止时释放；不桥接物理网卡。
- 入房后用 `status --json` 或 `doctor` 查看 `lan.ready`、MAC、IPv6、MTU。MAC 与控制端绑定成功才就绪；游戏选择此虚拟网卡或其虚拟 IP。是否自动出现房间仍取决于游戏的接口选择与已配置发现规则。

游戏网卡 MTU 固定 **1500**，无需玩家手调；内部分片、丢包取舍及协议边界见 [MTU 评审](docs/game-network.md#mtu-评审)。

## 操作

| 命令 | 行为 |
|---|---|
| `room invite --expected-revision <revision>` | 生成最长 30 分钟且不超过房间到期的新邀请码，旧码立即失效；可多人使用至房满 |
| `room kick <device-id> --expected-revision <revision>` | 踢出目标，并禁止该账号的所有设备再次用邀请码进入本房；撤销旧证书 |
| `room transfer <device-id> --expected-revision <revision>` | 转让房间管理权，不迁移游戏进程或存档 |
| `room leave` | 退出房间、停止本机网络；房主保留房间管理权 |
| `room close --expected-revision <revision>` | 房主关闭房间，撤销所有成员凭据 |
| `room … --room <id>` | 显式选择管理的房间；房主离房后管理时使用 |
| `games`、`games --json` | 读取服务端已启用游戏、配置端口和资料 |
| `status --watch`、`peers`、`ping <成员>` | 分别显示控制状态、Nebula 路径与真实探测结果；支持 `--json` |
| `doctor` | 查看 LAN 就绪信息、网卡、凭据、控制状态和探测结果 |
| `nlroom-service service install/uninstall` | 注册或删除 Windows 服务，需要管理员 |

`--json` 返回统一响应外壳，业务值在 `data`；退出码 0 成功、2 用户或条件错误、3 暂不可达或系统故障、4 操作结果待确认。写命令可用 `--command-id <原ID>` 恢复，`get-operation <ID>` 查询原收据；不能通过自动换 ID 重试未知结果。房间管理需传最后读取的 `--expected-revision`，建房需游戏的 `--game-revision`。

`network-stop` 持久暂停本机游戏网络并保持成员心跳；`network-retry` 核对当前成员后恢复网络。`room invite-info` 查询邀请有效性，`room invite-revoke` 停止邀请，`room owner-join --room <ID> --expected-revision <revision>` 让房主重新成为成员。`account status/devices` 查询自身占用与授权设备；接管需显式传期望房间、设备和成员版本，接管成功后再完成原入房。

每账号同时一台设备联机、每设备同时一房，新房间默认 4 人，有效期 24 小时；房主离线不关闭房间。填写昵称创建访客，身份由本机私钥证明，昵称不唯一且不能用于找回账号。绑定 OIDC 后成为正式用户，用户 ID、房间和房主身份保留；不同账号不自动合并。管理台使用独立的管理员账号密码。默认地址池 `10.203.0.0/16`，页面初始化时可改，运行中不能直接换池。

证书最多 10 分钟，剩余约 7 分钟开始续签。控制失联期间不接受新操作；游戏流量受最近心跳后 45 秒授权及当前凭据截止时间约束，实际可用时间也取决于对端和 relay。端口与 LAN 策略变化会受控重启 Nebula，短暂重连，这是规避固定上游版本防火墙热更新竞争的措施。

Go 开发归档可从原解压目录运行 `nlroom-update.exe remove` 卸载；不要从将被删除的安装目录运行助手。默认保留设备身份，`--purge` 同时清除身份。后台退出会关闭隧道和 TAP（基础设施释放 Nebula TUN）。

## Linux 桌面与客户端开发

Linux 桌面完整包需按下方步骤单独构建，包名为 `dist/desktop/nlroom_0.3.0_amd64.deb`。在 Ubuntu 22.04/24.04、Debian 12/13 上由 apt 安装依赖；首次安装需显式绑定玩家：

```sh
sudo apt install ./nlroom_0.3.0_amd64.deb
sudo nlroom-setup --owner "$USER"
nlroom
```

后续退出 GUI，再用相同 apt 命令安装新版 deb。升级停止旧后台，保留 UID 和身份，并重启已启用的后台；就绪失败会让软件包配置报错。修复后执行 `sudo dpkg --configure nlroom`，或使用兼容的旧 deb 显式降级。系统管理员禁用的服务不会自动启用。`sudo apt remove nlroom` 卸载程序，保留 `/var/lib/nlroom` 与 `/etc/nlroom`；purge 同样保留设备身份，销毁身份须自行明确处理这两个目录。

Linux Go 归档包含 `nlroom-cli` 和 `nlroom-service`，用于开发和隔离回归。显式 `--state-dir` 下 socket 为 `0600`，CLI 和后台必须使用同一系统用户。桌面版使用 root 服务与绑定安装用户 UID 的独立 socket，权限设计见 [桌面架构](docs/client.md#桌面架构)。

在具有 TUN 权限的隔离开发环境，两个终端使用相同用户执行：

```sh
# 终端一，前台运行后台服务
nlroom-service --state-dir /state/client daemon
# 终端二
nlroom-cli --state-dir /state/client init --server https://room.example.com --name 玩家甲
nlroom-cli --state-dir /state/client status --json
```

## 玩家账号

首次使用选择语言，再登录账号或填写昵称创建访客。设置 → 账号可绑定账号、登录已有账号、断开其他设备联机或退出正式账号；登录使用系统浏览器，凭据不进入桌面页面。未绑定访客丢失本机身份或切换账号后无法凭昵称找回，建议先绑定。窗口退出和账号退出分别处理。

CLI 使用 `account link` 或 `account login --server <origin> --name <昵称>` 获取登录地址，在浏览器完成并确认设备后运行 `account poll`；`account cancel` 停止等待，`account logout` 撤销正式账号当前设备，`account takeover` 断开此账号其他设备的房间连接。账号绑定和服务端用户管理见 [账号登录配置](docs/deployment.md#账号登录配置)。

## 开发与构建

管理台使用 React 19、TypeScript 和 Vite，Node.js 24 LTS 负责开发与构建，Go 同源提供编译后的页面与 API，生产环境无需运行 Node.js。前端位于 `internal/control/adminweb/`，依赖由 `package-lock.json` 锁定。官网位于 `internal/control/siteweb/`，使用共享 Go HTML 模板和本地静态资源，随 Go 编译嵌入，无独立构建步骤。Go 最低版本由 `go.mod` 声明，CI/容器使用 1.26.8。开发约束见 [AGENTS.md](AGENTS.md)。

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

`go test ./scripts/architecture` 单独检查跨平台包依赖和文件索引；索引检查覆盖 Git 已跟踪及未忽略的新文件，源码副本缺少 Git 元数据时跳过。约束见 [代码组织](docs/architecture.md#代码组织)。

Go 编译前须构建前端，`go:embed` 嵌入 `internal/control/adminweb/dist/`；构建目录与 `node_modules` 不入库。发布脚本、源码 Dockerfile 与 CI 已自动执行此步骤。`npm --prefix internal/control/adminweb run dev` 启动 Vite，开发时可访问 `/preview.html` 查看使用演示数据的管理台；预览入口不进入生产构建。完整登录与初始化验收使用 Go 提供的随机管理入口，以满足已配置的同源 Origin。

Windows 上构建 Windows/Linux amd64、arm64 归档（PowerShell 5.1+）：

```powershell
.\scripts\build.ps1 -Go 'C:\Program Files\Go\bin\go.exe'
# 只更新服务端归档时：
.\scripts\build.ps1 -Targets 'linux/amd64','linux/arm64'
```

版本分别维护在 `internal/model/version.go` 的 `ControlVersion`、`NodeVersion`、`ClientVersion`；客户端同时更新 desktop 的 npm/Tauri/Cargo 程序版本，管理台 npm 程序版本随控制面更新。协议版本独立维护。构建按组件写入 `dist/control/<版本>/`、`dist/node/<版本>/`、`dist/client/<版本>/`，归档名包含组件、版本、系统和架构；`-Components client` 或 `-Components node` 可单独构建。控制面构建同时准备其固定节点版本的 amd64/arm64 原生安装资源。

客户端归档仅含 CLI、后台及许可，Windows 另含安装入口；控制面与节点使用各自 Linux 归档，控制面附部署模板和同源节点安装包。每个组件目录独立生成 SHA256SUMS。完整桌面安装包仍写入 `dist/desktop/`；程序尚未由 NodeLane 代码签名证书签名。构建不安装服务、不创建云资源。

构建后用 `python scripts/check-release.py dist` 检查归档的 SHA256、内容、架构和 Linux 执行权限。

桌面前端用 `npm --prefix desktop ci` 安装依赖，`npm --prefix desktop run dev` 启动；`http://127.0.0.1:1420/preview.html?state=room/setup/login/account/signed-out/error` 提供标明示例的预览。`npm --prefix desktop run build` 构建后，可用 `cargo build --manifest-path desktop/src-tauri/Cargo.toml --locked --release --features custom-protocol` 仅构建 GUI。

桌面完整包需要 Rust 1.95、Node.js 24、Windows NSIS 或 Linux WebKitGTK 4.1 系统依赖。Windows 打包会下载并校验固定的 TAP 驱动、网卡工具及对应源码，缓存位于 `.local/desktop-drivers`；Linux 交叉打包另需 `msitools`，构建镜像已包含。GUI、Rust、Go 和传入发布目录的版本必须一致；构建器校验架构并记录摘要，产物附带 SHA256。在对应平台执行：

```sh
python scripts/desktop/build.py --platform windows --arch amd64 --release dist/client/0.3.0/nodelane-room-client-0.3.0-windows-amd64
python scripts/desktop/build.py --platform linux --arch amd64 --release dist/client/0.3.0/nodelane-room-client-0.3.0-linux-amd64
```

`scripts/desktop/Dockerfile` 提供 Ubuntu 22.04 构建环境、Windows 交叉编译工具和原生 WebView 验收工具。`--skip-web` 仅复用已构建并检查的前端资源。当前生成的 EXE 为未签名测试包，deb 尚未进入签名软件仓库；构建不安装宿主服务、不发布产物。ARM64 参数用于相应工具链，未通过 ARM 真机验收。

桌面检查：`npm --prefix desktop test`、`cargo test --manifest-path desktop/src-tauri/Cargo.toml --locked`、`python scripts/desktop/test_package.py`；原生安装事务与 Windows API 检查纳入 `go test ./internal/update`，不安装宿主服务或驱动。打包后可构建上述 Dockerfile 为 `nodelane-desktop-build:local`，再运行 `python scripts/desktop/test_live.py --package dist/desktop/nlroom_0.3.0_amd64.deb`，通过真实 WebView、普通用户 socket、隔离 HTTPS/数据库与 Nebula 验证流程。它仅安装到临时容器，结束后清理；不代替宿主 systemd 或 Windows 服务验收。

## 实时网络监控

管理台展示节点、房间和成员的实际链路、RTT/丢包、流量趋势及观察到的出口和地区；无有效观测时显示未知。采样与统计口径见 [架构说明](docs/architecture.md#监控)，Linux 采集权限、GeoIP 自动下载及本地库配置见 [部署指南](docs/deployment.md#监控与-ip-归属地)。

## Docker 双客户端回归

在源码根目录运行 `python scripts/test-docker.py`。需要 Python 3.10+、Docker Compose 和提供 `/dev/net/tun` 的 Linux Docker 引擎（包括满足条件的 Docker Desktop）。首次运行需要下载镜像与 Go 依赖，不使用 `dist` 产物。加 `--verify` 会额外在 Linux 容器中使用独立 PostgreSQL 测试库执行 `go vet ./...`、`go test -count=1 ./...`、`go test -race -count=1 ./...`；race 失败会保留失败状态和日志。

两个玩家容器分别连接隔离的 `172.30.81.0/24`、`172.30.82.0/24` 网络；容器内还配置对侧网段不可达路由。额外三个容器提供测试控制面、PostgreSQL、固定补丁版本的 Nebula lighthouse/relay。基础设施关闭 IP 转发，不发布宿主端口，脚本不调用宿主防火墙或路由配置命令。测试网段须与现有 Docker 网络不重叠；如果 Docker 报冲突，在 `deploy/test/compose.yaml` 和测试脚本中同步选择空闲网段。

脚本先证实底层 TCP/UDP 双向不通，再通过真实 TAP/Nebula 中继测试注册、邀请、房间、默认拒绝、跨房隔离、配置替换和撤销；验证 IPv4/IPv6、广播/组播、大 UDP 报文与 LAN 策略收窄时，将测试容器底层 MTU 设为 1280、游戏 TAP 保持 1500。测试使用 socket 回显与发现夹具，不运行游戏本体；此拓扑没有模拟运营商 NAT，也不覆盖 Windows 驱动/服务验收。

每次运行创建唯一 Compose 项目。临时密钥、身份和数据库保存在容器临时存储中，只有 HTTPS 公钥证书共享给客户端；邀请码和登记令牌不写入日志。退出时清理该次容器、网络、证书卷和带本次唯一标签的测试镜像，检查结果与构建/验证日志保存在 `.local/nodelane-test-*/`。源码配置位于 `deploy/test/`，不会放入发布包。

部署模板冒烟使用 `python scripts/test-deploy.py`；加 `--host` 测试复用现有设施的精简编排。可加 `--root dist/control/0.3.1/nodelane-room-control-0.3.1-linux-amd64 --node-root dist/node/0.2.1/nodelane-room-node-0.2.1-linux-amd64` 验证发布包，或 `--images --pull` 验证仓库发布镜像。`--extended` 额外验证真实十分钟证书续签与断控到期。它测试单实例页面初始化、数据库与 CA 保存、Caddy 内部测试 HTTPS、令牌签发、节点登记、真实 TUN、持久化身份和控制容器重建恢复，不开放宿主端口；不代替宿主网关/端口连通性、现有反代配置、公网证书和 Windows 真机验收。

源码入口见 [文件索引](docs/files.md)，调用方与权限分工见 [架构说明](docs/architecture.md)，接口见 [OpenAPI](docs/openapi.yaml)。当前不包含支付、多账号合并、管理员 OIDC、多管理员角色、TOTP 或云资源自动创建；LAN 兼容范围和游戏验收要求见游戏网络文档。
