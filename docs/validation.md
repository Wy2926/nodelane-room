# 验证记录

仅在验证、排错或发布任务中读取。结果只对应当时源码或产物，跳过不算通过；新增记录注明提交或产物摘要、环境、结果和脱敏证据。历史记录未注明的提交不补推。`.local/` 不随源码保存，缺失日志仅保留原路径追溯。

## 当前源码与产物

当前源码为访客/OIDC 账号、默认 4 人房间、统一 Ethernet LAN、客户端更新、数据库结构 6、本机 IPC 3、interaction-1 契约；控制面和客户端源码版本 0.3.0，节点 0.2.1。最近已发布制品仍为控制面 0.2.2、节点与客户端 0.2.1，本次未发布新制品。下表保留原检查结果，后续改动不能据此自动判定通过；网络设计见 [游戏网络](game-network.md)。

| 对象 | 适用范围 |
|---|---|
| 当前 LAN 源码 | 单播、广播/组播、帧授权和分片已实现；组件和 Linux TAP 检查见下节，游戏本体仍未验收 |
| 当前完整安装包 | Windows amd64 0.3.0 已重建为原生覆盖安装，TAP 改为客户端启动时按需安装，移除安装器 WebView2 与 PowerShell 依赖；安装包未签名，完整安装与驱动加载仍待真机验收 |
| 历史 0.2.0 安装包与镜像 | 使用旧数据面，不能与当前客户端混用；发布源码提交和镜像摘要见 [IMAGES.txt](../deploy/IMAGES.txt)，旧包/WebView 结果只列于历史证据 |

## GUI 地址设置与 TAP 启动检查（2026-09-12）

在原生安装工作区上移除 GUI 初始化与账号登录的服务端地址输入；新身份使用线上控制端，已有身份沿用后台配置。TAP 检查改用 PnP 实际设备的驱动登记，排除已移除设备的连接残留，保留读取失败原因，并检查已登记网卡是否可被系统网络接口查询到；官方驱动 ID、最低版本及卸载 GUID/名称校验保持。

| 检查 | 实际结果 |
|---|---|
| Windows Go | Go 1.27.0：gofmt、全量 vet、普通测试通过，含真实 Nebula 进程内集成；最终 TAP 改动另经 update vet、update 与 architecture 测试复查。临时用户注册表覆盖其他驱动、低版本、读取失败及孤立连接登记；实际 PnP 枚举为只读，不安装或打开 TAP 数据句柄。[全量](../.local/tap-fix-tests.log)、[最终定向](../.local/tap-fix-targeted-tests.log) |
| 桌面 | Node 24.21.0：50 项测试及生产构建通过，覆盖无地址输入、固定初始化地址和已有账号控制端保留。Rust 1.95.0：6 项通过，1 项已安装服务往返检查跳过。[前端](../.local/tap-fix-desktop-tests.log)、[Rust](../.local/tap-fix-rust-tests.log) |
| 打包检查 | Python 6 项通过，7 项 Debian 检查按平台跳过；WebView 夹具的隔离控制端改由 CLI 初始化，仅检查语法，未运行 WebView 联机流程。[打包检查](../.local/tap-fix-package-tests.log) |
| 安装包 | amd64 Go 归档、原生 GUI release 与 NSIS 3.11 完整包重建通过；首次打包未找到 PATH 中的 NSIS，使用已有工具重新打包通过。[GUI 构建](../.local/tap-fix-installer-build.log)、[最终打包](../.local/tap-fix-package-build.log)、[摘要](../.local/tap-fix-artifacts.json) |

本轮未配置 Windows 测试数据库，数据库用例跳过；未重跑 Linux 数据库/race 与 Docker TAP 拓扑。用户已卸载报错客户端，原故障状态未完整复现；新包的 Windows 初装、UAC、TAP 加载、卸载与双机 NAT/Minecraft 仍须实机复测。

本地安装包 `dist/desktop/nlroom-0.3.0-windows-amd64-setup.exe`，SHA256 `05218287fc0b33149f23e26ee7e9627ff8309864d6d3bd230972effa6b05e726`。本轮未安装宿主服务或驱动、未发布；其他架构未重建。

## Windows 原生安装简化（2026-09-12）

安装、更新和卸载改用 Go 原生助手，删除四个运行时 PS1 及对应模拟测试；保留验包、停服务等待、权限和身份绑定。Windows 采用完整包覆盖，中断后重跑安装包修复，移除自动回滚、旧版备份和开机恢复；TAP 在已安装 GUI 启动时检查，仅缺失时提权安装。WebView2 为已有环境前提。操作见 [Windows 客户端](../README.md#windows-客户端)。

| 检查 | 实际结果 |
|---|---|
| Windows Go | Go 1.27.0：gofmt、vet 和全量测试复查通过；最后的安装改动经 update 与 architecture 定向复查。覆盖载荷路径/摘要/架构、拷贝中断后重装、旧文件损坏修复、身份保留、临时注册表和子进程错误传递。未配置 Windows 测试数据库。[全量复查](../.local/native-install-go-final.log) |
| 签名与原生 API | 实际核验随包 TAP sys/cat 的 Microsoft 签名、tapctl 的 OpenVPN 签名；错误发布者和篡改文件被拒绝。临时目录快捷方式、无提权子进程及注册表测试通过。[签名](../.local/native-install-tap-signatures.log) |
| 桌面 | Node 24.21.0：49 项测试和生产构建通过；Rust 1.95.0：6 项通过，1 项已安装服务往返测试跳过。正式 GUI release 构建通过，含启动 TAP 检查与错误详情。 |
| 安装包 | Python 6 项打包检查通过，7 项 Debian 检查按平台跳过；NSIS 3.11 严格编译通过。amd64/arm64 Go ZIP 摘要、完整载荷、PE 架构与无 PS1 检查通过。完整 EXE 仅构建 amd64，未执行安装。[构建](../.local/native-install-desktop-build.log)、[制品](../.local/native-install-artifacts.json) |
| Linux 数据库、并发与真实 TAP | 独立 PostgreSQL：vet、全量测试及 `go test -race -count=1 ./...` 通过。双客户端真实 Nebula TAP/中继、跨房拒绝、撤销、策略变化、IPv4/IPv6、32KB UDP、广播/组播和低 MTU 通过，测试容器、卷与网络清理通过。[结果](../.local/nodelane-test-20260912-082945-24bb4d/results.json) |

本轮 Windows 全量测试曾触发 Nebula RIO 的 `cq is corrupt` 崩溃，复查通过，但不视为修复；固定上游与补丁未改动。[失败日志](../.local/native-install-go-test.log)。一次 Docker 拓扑运行在初始探测返回 `local_peer_unreachable` 后中止，完整复查通过。[该次结果](../.local/nodelane-test-20260912-082546-c877b6/results.json)。

本地产物 `dist/desktop/nlroom-0.3.0-windows-amd64-setup.exe`，SHA256 `302d97ce5085d955a58951a0807afa7da4d85681d85c8299076442a407b5e4d2`。未发布、未安装宿主服务或驱动。Win10/Win11 真机初装、覆盖升级、中断重装、SID/UAC、TAP 加载与卸载，以及双机 NAT/Minecraft 仍未验收；未运行全组件发布归档检查。

## 房间布局与后台测量（2026-09-12）

基于 `d3b32eb` 的后续工作区：房间成员共享列宽，游戏与操作移入右栏；个人菜单承接退出方式。窗口仅保留关闭，诊断精简为连接状态和系统检查。后台探测独立于控制请求，每 5 秒采样并滚动保留最近 30 秒；协议字段不变，统计口径已更新 OpenAPI。

| 检查 | 实际结果 |
|---|---|
| 桌面 | Node 24.21.0：49 项测试和生产构建通过。覆盖菜单退出失败不关窗、受限账号入房、真实零值、全丢包、30 秒过期及诊断移除项。[测试](../.local/gui-refine-desktop.log)、[构建](../.local/gui-refine-build.log) |
| Windows Go | Go 1.27.0：gofmt、vet、全量测试通过；未配置 Windows 测试数据库。[测试](../.local/gui-refine-go.log)、[vet](../.local/gui-refine-vet.log) |
| Linux 数据库与并发 | Go 1.26.8、独立 PostgreSQL 18.6：vet、全量普通测试和 `go test -race -count=1 ./...` 通过，含真实 Nebula 隔离、Ethernet、P2P 与中继。新增控制请求阻塞时自动探测、取消不计丢包和 30 秒窗口测试通过。[记录](../.local/gui-refine-linux.log) |
| 原生 GUI | Rust 1.95.0：5 项测试通过，1 项需要已安装 Go 服务的往返测试跳过；`custom-protocol` release 构建通过，正式 React 资源已嵌入。仅 GUI 产物 `dist/desktop/gui-react/nlroom.exe`，SHA256 `4417c9f51e716c655be811d470c2de6ddca2761d2593a4888053004a5555403f`；运行需配套当前后台。[Rust 检查](../.local/gui-git-rust-test.log)、[构建](../.local/gui-git-native-build.log) |
| 浏览器 | 1120×760、760×560 与 390×844 检查通过；中英文表头与各成员行列坐标相同，窄屏只滚动表格，不撑宽页面。检查个人菜单 Escape/外部点击、成员菜单及诊断结果。[房间](../.local/gui-refine-room.png)、[个人菜单](../.local/gui-refine-profile.png)、[诊断](../.local/gui-refine-diagnostics.png) |

浏览器使用明确标注的示例数据；未运行 Windows 服务/驱动与原生窗口、托盘真机验收，未运行双机 NAT/Minecraft、Docker 双客户端 TAP 拓扑和安装器测试，未构建发布完整安装包。临时测试数据库、网络与缓存卷已清理。

## 客户端重建与建房限制（2026-09-12）

基于 `d3b32eb` 的工作区：按选定的第三版暖白设计重建房间列表、入房侧栏、详情及共享样式，删除旧游戏库、旧主题样式与背景素材。账号 `disabled` 改为只限制建房，数据库结构仍为 6。

| 检查 | 实际结果 |
|---|---|
| Windows Go | Go 1.27.0；`gofmt`、`go vet ./...`、`go test ./...` 通过。未配置 Windows 测试数据库，数据库用例在下方 Linux 实际执行。[测试](../.local/gui-rebuild/go-windows.log) |
| Linux PostgreSQL 与 race | Go 1.26.8、独立临时 PostgreSQL 18.6；vet、全量普通测试和全量 race 通过。覆盖受限账号的 OIDC 登录、会话续取、入房、心跳、证书续签、成员快照、幂等建房拒绝、权限恢复和删除撤销；真实进程内 Nebula 隔离、Ethernet、P2P 与中继测试通过。临时容器、网络与缓存卷已清理。[记录](../.local/gui-rebuild/go-linux.log) |
| 前端 | 固定 Node 24.21.0：客户端 45 项、管理台 14 项测试与两端构建通过。首次默认 Node 22 的测试结果不作为固定环境验收。[客户端](../.local/gui-rebuild/desktop-node24-tests.log)、[管理台](../.local/gui-rebuild/admin-node24-tests.log) |
| GUI | 浏览器验证限制建房但可入房、正常建房与临时邀请、离房确认、设置、诊断、离线设置和 760×560 最小窗口；设计对照和截图见 [QA](../.local/gui-rebuild/design-qa.md)。 |

本轮未重新运行 Docker 双客户端 TAP 拓扑，未运行 Rust/安装器测试；原生桥接与安装器源码未变。Windows 服务、SID/Named Pipe、TAP 驱动、托盘与窗口原生行为、双机 NAT/Minecraft 仍需真机验收。未构建发布完整安装包。

## interaction-1 实施验收（2026-09-12）

基于 `ef1475d` 的当前工作区，按四批次完成业务码与安全错误、事务收据与当前授权、本机操作恢复及桌面流程、管理台与节点异步响应。HTTP 仍为 `/v2`，新增 `interaction-1` 契约；本机 IPC 为 3。数据库结构保持 6，Nebula 固定版本和已授权补丁未变。

| 检查 | 实际结果 |
|---|---|
| Windows Go | Go 1.27.0；`gofmt`、`go vet ./...`、`go test ./...` 通过。Windows 未配置测试数据库，数据库用例在下方 Linux 执行；受保护 ProgramData 的服务恢复用例在 Windows 跳过。[测试](../.local/go-full.log)、[vet](../.local/go-vet.log) |
| Linux PostgreSQL 与 race | Go 1.26.8、独立 PostgreSQL 18.6；完整普通测试与 `go test -race -count=1 ./...` 通过，实际运行数据库用例。覆盖拒绝保存点回滚、跨副本收据、过期拒绝、重放权限及邀请脱敏、账号/设备/节点状态、原始字节恢复与暂停非阻塞。[独立 race](../.local/linux-contract-tests.log) |
| 真实 Nebula 与 TAP | `scripts/test-docker.py --verify` 通过：隔离双客户端、真实 relay/RTT、TCP/UDP、跨房拒绝、离房/踢人撤销、策略收窄、IPv4/IPv6、32KB UDP、广播/组播及 MTU 1500/1280 场景；同时通过 Linux vet、全量普通及 race 测试，测试资源清理通过。[完整结果](../.local/nodelane-test-20260912-023757-2b30d0/results.json) |
| 桌面与管理台 | 按项目固定 Node 24.21.0 验证：桌面 41 项、管理台 14 项测试与两端构建通过；包含接管的新子步骤、未决操作互斥、跨实例旧回复丢弃、版本条件和本地化覆盖。[桌面](../.local/desktop-tests.log)、[管理台](../.local/admin-tests.log) |
| Rust 与安装脚本 | Rust 1.95.0：5 项通过，真实已安装服务往返的 1 项跳过；安装文件交换/恢复模拟 12 项通过。UAC 取消已单独分码，未实际触发提权。[Rust](../.local/rust-tests.log)、[安装模拟](../.local/installer-contract-tests.log) |
| 契约与代码组织 | OpenAPI 生成一致性、包边界、文件索引及 `git diff --check` 通过；清理失效的旧收据校验，安装和回归入口同步协议 3。 |

过程中实际发现并修正了复用解码对象保留旧字段、SSE reset 提交空快照、节点首次登记缺少绑定代次、将请求代次冲突缓存为永久身份终态等问题；上述记录对应最后通过的运行。旧客户端/旧响应不提供兼容路径，未迁移或清理旧数据库。

Windows 真实服务、安装用户 SID/Named Pipe、TAP 驱动加载、UAC/完整安装更新，以及双机 NAT、Minecraft 和真实外部 OIDC 提供方仍须真机验收。容器流量与协议夹具不替代这些验收；未构建发布新的完整安装包或部署生产服务。

## 客户端更新验证

2026-09-11，基于 `6d895fa` 的客户端更新实现；配置与发布步骤见 [客户端更新](updates.md)。未创建生产密钥、对接真实存储、修改线上数据库或安装宿主服务。

| 检查 | 实际结果 |
|---|---|
| Windows Go | Go 1.27.1，`go vet ./...` 和最后一次 `go test ./...` 通过，使用独立 PostgreSQL 实际执行数据库用例。此前本轮全量运行触发已知 Nebula `RIOConn.WriteTo`（`udp_rio_windows.go:298`）空指针，复查未复现；未修改或修复上游数据面。[最终测试](../.local/update-validation/windows-go-test.log) |
| Linux Go | Docker Go 1.26.8、独立 PostgreSQL 18.6；`go vet ./...`、`go test -race -count=1 ./...` 通过。含数据库事务、真实进程内 Nebula、签名篡改/过期/防回退、离线签署及根轮换；容器未授予宿主 TAP/systemd 权限 |
| 强制规则与版本 | 覆盖到期撤销既有证书/成员、旧版本拒绝创建、升级报告后重新授权、无房间上报、GUI 变化、并发策略修订、清除规则后的旧修订拒绝、源切换可用性、凭据只写与 S3 SigV4 协议夹具 |
| 前端与 Rust | 管理台 14 项、桌面 38 项测试及构建通过；Windows Rust 4 项通过，需已安装服务的 1 项跳过；覆盖安装确认、下载中禁用安装及编辑中的修订保持 |
| 安装脚本 | Windows PowerShell 5.1：12 项安装事务（含目录交换前/后中断恢复）、8 项卸载、3 项状态管道测试通过；SCM/驱动操作为模拟。Linux 容器 Python 13 项打包及 Debian 生命周期测试通过 |
| 契约与部署 | OpenAPI 生成一致性、包边界、文件索引和 Compose 配置校验通过 |

真实 R2/AWS S3/MinIO 的权限、上传及公网源，Windows UAC/计划任务/驱动与完整 EXE 安装，Ubuntu/Debian systemd/APT 的完整 deb 更新、断电和回退，以及双机 NAT/Minecraft 尚未验收。0.3.0 完整签名安装包与镜像尚未构建发布。上述 Windows RIO 崩溃仍属发布前需要处理的已知问题，不以一次重跑通过视为修复。

## 控制面 0.2.2 OIDC 修复发布（2026-09-11）

控制面镜像 `sha256:d979e84c6b6ce84798cc76053ecd7e2497322319ae478224369d729930fa5a4c`，amd64/arm64 均已推送并核对远端摘要；节点、客户端保持 0.2.1。构建源码清单 SHA256 `93a1f06a90a1b69bf9ba26505bceb4fa3f77f1f41aca07d77d732c005e70f88c`，记录见 [镜像清单](../deploy/IMAGES.txt)。未更新线上容器。

| 检查 | 实际结果与证据 |
|---|---|
| OIDC 浏览器与授权边界 | 本地 IdP 与真实浏览器复现旧页面的 `Origin: null`；修复后确认成功并领取设备会话，Referer 仅包含 origin。8 种错误来源/Cookie/CSRF 请求拒绝，正确确认通过。[浏览器](../.local/oidc-confirm/browser-after.log)、[回归](../.local/oidc-confirm/confirmation-security.log) |
| Go 与独立 PostgreSQL | Windows Go 1.27.1、Linux Go 1.26.8 的 vet 和全量普通测试，以及 Linux 全量 race 通过；数据库用例实际运行，包含真实 Nebula Ethernet、隔离、撤销、中继和 P2P 集成。[Windows](../.local/release-0.2.2/windows-test.log)、[Linux](../.local/release-0.2.2/linux-test.log)、[race](../.local/release-0.2.2/linux-race.log) |
| 发布镜像 | 双架构程序实际运行并校验版本、二进制摘要及节点下载清单；ARM 经仿真。控制面 0.2.2 与节点 0.2.1 的隔离 host 编排通过 HTTPS 初始化、登记、重建恢复和撤销。[程序](../.local/release-0.2.2/image-verification.json)、[部署](../.local/release-0.2.2/deploy-test.log) |
| 归档、管理台与安装脚本 | 分组件归档校验通过，节点 0.2.1 产物摘要不变；管理台构建和 13 项测试通过。Windows 安装 10 项、卸载 8 项、状态管道 3 项、TAP 14 项模拟回归通过；Python 打包 6 项通过，6 项 Linux 生命周期按平台跳过。[归档](../.local/release-0.2.2/archive-check.log)、[管理台](../.local/release-0.2.2/admin-tests.log)、[安装脚本](../.local/release-0.2.2/installer-tests.log) |

本轮未执行线上 Logto 登录、Windows 服务/驱动真机安装、双机 NAT/Minecraft、外网 Steam 和 MMDB 样本检查；客户端安装包未重新构建。

## 0.2.1 Windows TAP 校验修复（2026-09-11）

在下方安装包修复基础上，修复两处实际安装问题：注册表改为先枚举名称，避开受保护的非驱动 `Properties` 项；安装器和 Go 后台同时接受官方 INF 的 `root\tap0901`、`tap0901`，仍校验专用网卡 GUID。先前包 SHA256 `149dd6e3534f78a538a6250b36c42019cf27a37bd4ef41cb21c4a45ba7fe99e7` 只修复第一处，用户实测在第二处失败。Windows amd64/arm64 客户端后台已重建，GUI 和驱动保持不变。未操作宿主服务、网卡或注册表权限；测试仅创建并清理临时 HKCU 键。

| 检查 | 实际结果与证据 |
|---|---|
| 本机只读复现 | Windows PowerShell 5.1.26100.7705 普通用户下，原查询在网卡类 `Properties` 项复现访问拒绝；新查询成功读取 25 个驱动项。[日志](../.local/tap-registry-fix/registry-read.log) |
| 脚本与打包回归 | TAP 14 项、安装事务 10 项、卸载 8 项、状态管道 3 项通过；真实创建 ID 的新回归在修复前复现误判。服务/网卡操作均为模拟。Python 打包 6 项通过，6 项 Linux 生命周期用例按平台跳过。[修复前](../.local/tap-component-fix/before.log)、[通过日志](../.local/tap-component-fix/script-tests.log) |
| 后台网卡识别 | 临时 HKCU 注册表 7 个用例通过，覆盖两个官方 ID、大小写、其他 GUID/驱动、近似和空 ID；测试键已清理。Windows 两份开发归档已重建，归档摘要与架构检查通过。[注册表](../.local/tap-component-fix/registry-test.log)、[构建](../.local/tap-component-fix/client-build.log)、[归档](../.local/tap-component-fix/archive-check.log) |
| 完整安装包 | 同名 Windows amd64 0.2.1 EXE 已替换，15,624,110 字节；SHA256 `84ed70edc1f6f52d7758d980d1167317f7ec6154712113b4dbb95beccf360e9b`。NSIS 编译、7z 完整性、613 个 payload 文件摘要、修复脚本及 GUI/新后台一致性通过；TAP 摘要、CAT/SYS/tapctl/WebView2 签名均有效。[包校验](../.local/tap-component-fix/installer-verification.json)、[归档](../.local/tap-component-fix/installer-archive.log)、[签名](../.local/tap-component-fix/signatures.json) |
| Go 检查 | 修改文件 gofmt、vet、全量普通测试与文件索引检查通过，包含真实 Nebula Ethernet、隔离/撤销、中继和 P2P 集成；未配置测试数据库，数据库用例跳过，本轮未运行 Linux/race。此前两次测试在 `TestNebulaNativeRelay` 的 Windows `RIOConn.WriteTo`（`udp_rio_windows.go:298`）发生 `0xc0000005`，此次未复现，问题未修复。[vet](../.local/tap-component-fix/go-vet.log)、[全量测试](../.local/tap-component-fix/go-tests.log)、[此前崩溃](../.local/tap-registry-fix/engine-recheck.log) |

完整 UAC 安装、驱动加载、卸载和双机 NAT/Minecraft 仍未验收；上述包校验不代表 Windows 网络链路已验收。

## 0.2.1 Windows 安装包修复（2026-09-11）

在下方发布源码基础上修改安装脚本、驱动打包和日志回传；客户端版本保持 0.2.1。按用户要求移除新增的旧服务兼容分支，并手动停止、删除宿主原有 `nodelane.exe` 服务，保留程序文件和身份数据；服务项与进程已确认不存在。[移除记录](../.local/tap-package/service-removal.log)。未安装新服务或驱动，控制面与节点镜像未改动。

| 检查 | 实际结果与证据 |
|---|---|
| 完整安装包 | `dist/desktop/nlroom-0.2.1-windows-amd64-setup.exe`，15,630,001 字节；SHA256 `6d611dc7e0f5fa423c324e2a4ef15687611ee49b7f3092a7aaca5c5dc15f4cb8`。NSIS 编译、7z 完整性、613 个 payload 文件摘要和 GUI/后台一致性通过。[校验](../.local/tap-package/installer-verification.json)、[归档](../.local/tap-package/installer-archive.log) |
| 内置驱动 | TAP-Windows6 9.27.0 的 CAT/SYS Microsoft 签名、OpenVPN 2.6.22 tapctl 签名和 WebView2 签名均有效；摘要与 PE 架构通过检查。对应完整源码和许可随包提供。[签名](../.local/tap-package/signatures.json) |
| Windows 脚本 | Windows PowerShell 5.1 下安装事务 10 项、卸载 8 项、状态管道 3 项、TAP 操作 11 项通过；服务和网卡操作均为模拟，状态管道使用真实普通用户子进程。覆盖驱动失败后的程序恢复、新建网卡登记与清理、拒绝操作其他 VPN 网卡及具体错误回传。Python 打包 6 项通过，6 项 Linux 生命周期用例按平台跳过。[日志](../.local/tap-package/script-tests.log) |
| Go 与开发归档 | Windows vet、全量普通测试和文件索引检查通过；未配置 Windows 数据库，数据库用例跳过，本轮未重跑 Linux/race。客户端四份开发归档已重新构建并核对摘要、版本和架构。[vet](../.local/tap-package/go-vet.log)、[测试](../.local/tap-package/go-tests.log)、[归档](../.local/tap-package/archive-check.log) |

此修复包替换了下方首次发布的同名 EXE；旧摘要仅供历史追溯。Windows UAC 下的完整安装、驱动加载和卸载仍未验收。

## 0.2.1 首次发布检查（2026-09-11）

基于 `5898fa2d5ef20b8f2681e0e021622ed7fffff7ee` 的未提交版本拆分与打包修改；源码清单、镜像摘要和旧版摘要见 [IMAGES.txt](../deploy/IMAGES.txt)。两份镜像已推送到 `docker.nodelane.net`，未更新线上容器。

| 检查 | 实际结果与证据 |
|---|---|
| 独立版本与归档 | 控制面/节点各两份 Linux 归档、客户端四份 Windows/Linux 归档通过 SHA256、版本、二进制隔离、架构及权限校验；控制面内置节点安装清单为 0.2.1。[归档](../.local/release-0.2.1-archive-check.log) |
| Windows Go | gofmt、vet、全量普通测试通过，包含进程内真实 Nebula 集成；未配置 Windows 测试数据库，数据库用例跳过。[vet](../.local/release-0.2.1-vet.log)、[测试](../.local/release-0.2.1-tests.log) |
| Linux 与独立 PostgreSQL | vet、全量普通测试和全量 race 全部通过，数据库用例实际执行；真实 TAP/Nebula 回归含 IPv4/IPv6、32 KB 数据报、MTU 1280、广播/组播、动态策略、跨房拒绝和撤销。[结果](../.local/nodelane-test-20260911-141921-558e83/results.json) |
| 前端与原生桥接 | 管理台 13 项、桌面 36 项测试及构建通过；Windows Rust 4 项通过，已安装服务烟测跳过。[管理台](../.local/release-0.2.1-admin-tests.log)、[桌面](../.local/release-0.2.1-desktop-build.log)、[Rust](../.local/release-0.2.1-rust-tests.log) |
| 部署与镜像 | 源码 Dockerfile、独立发布包的 host 编排、发布镜像三种部署冒烟全部通过，覆盖初始化、节点操作、容器重建恢复和撤销；所有临时资源已清理。[源码](../.local/nodelane-deploy-test-0ab06f65c9f0/results.json)、[发布包](../.local/nodelane-deploy-test-126a02d86c0f/results.json)、[镜像](../.local/nodelane-deploy-test-e9c2b237e68b/results.json) |
| 双架构发布 | 两份镜像的 amd64/arm64 程序均实际执行 `--version` 并核对二进制 SHA256；ARM 经仿真。推送后远端 OCI index 与本地构建摘要一致。[程序校验](../.local/release-0.2.1-image-verification.json)、[发布摘要](../deploy/IMAGES.txt) |
| Windows 完整安装包 | `dist/desktop/nlroom-0.2.1-windows-amd64-setup.exe`，13,446,861 字节；SHA256 `0b0e3bb80af1c08f173fcb501b659b5d01481a13f393fa1122e179e5f7066560`。NSIS 3.11 编译、归档完整性、610 个 payload 文件摘要、GUI/后台一致性和工具分离通过；WebView2 bootstrapper 的 Microsoft 签名有效。[包校验](../.local/release-0.2.1-installer-verification.json)、[解包](../.local/release-0.2.1-installer-archive.log) |
| 安装与打包测试 | Windows 临时目录/模拟 SCM 的安装 6 项、卸载 8 项通过；Python 打包 6 项通过，6 项 Linux 生命周期测试按平台跳过，含三端版本不同仍可打包客户端及客户端版本不符拒绝。[安装](../.local/release-0.2.1-installer-tests.log)、[卸载](../.local/release-0.2.1-uninstall-tests.log)、[打包](../.local/release-0.2.1-package-tests.log) |

首次 Windows NSIS 编译因语言文件路径失败，已改为脚本目录绝对引用并显式使用 UTF-8，随后完整编译和解包校验通过。本次完整 GUI 仅打包 Windows amd64；Windows ARM64 与 Linux 仅生成 CLI/后台归档，未生成对应 GUI 安装包。完整安装包未签名；Windows 真机服务/驱动、Linux 宿主 systemd、双机 NAT/Minecraft 和真实游戏本体仍未验收。本轮未运行外网 Steam 与 MMDB 样本检查，未运行完整原生 WebView 流程。

## 最近账号检查（2026-09-11）

基于 `2b8274853bcad35d7254634fc89914565f3f6b32` 的未提交工作区；代码清单统一 LF 后的 SHA256 为 `b6ffa26895f97af0ca2eaaa875d973cdccedc5304b5b7b9f0006fd7e09bd26fb`，明细见 [清单](../.local/users-source-manifest.json)。未生成或发布完整安装包。

| 检查 | 实际结果与证据 |
|---|---|
| Windows Go 与独立 PostgreSQL | gofmt、`go vet ./...`、全量普通测试通过；新增账号测试实际连接独立 PostgreSQL，覆盖同名访客、升级保留用户/房间、默认 4 人、拒绝合并、跨设备所有权与封禁、设备退出、管理员停用与证书撤销、旧续租缓存失效。[vet](../.local/users-windows-vet.log)、[测试](../.local/users-windows-test.log) |
| OIDC | 本地测试 IdP 执行 discovery、授权码、PKCE S256、JWKS/RSA 签名与浏览器确认；错误 issuer、audience、nonce、azp、过期令牌和错误领取证明被拒绝。尚未对接真实第三方租户 |
| Linux Go 与独立 PostgreSQL | vet、`go test -count=1 ./...`、`go test -race -count=1 ./...` 全部通过，含退出后重启与退出响应丢失恢复测试；不是跳过数据库用例。[普通测试](../.local/nodelane-test-20260911-134954-7492c4/test.log)、[race](../.local/nodelane-test-20260911-134954-7492c4/race.log) |
| 真实 Linux TAP/Nebula | 隔离双客户端经原生 relay 验证 TCP/UDP、IPv4/IPv6、32 KB 数据报、广播/组播、策略收紧、跨房隔离及离房/关房/踢人；TAP MTU 1500、底层 MTU 1280。[结果](../.local/nodelane-test-20260911-134954-7492c4/results.json)、[回归输出](../.local/users-docker-verification-final.log) |
| 桌面、管理台与桥接 | 桌面构建及 36 项测试、管理台构建及 13 项测试通过；Windows Rust 4 项通过，已安装服务烟测跳过。浏览器预览核对访客身份、绑定入口、账号/设备区分及凭据丢失提示；不是原生 WebView 或真实 OIDC 登录验收 |

首次新增管理台测试的错误文案断言失败，已修正并重新通过本机及容器构建。Windows 两项账号持久化测试因临时目录不满足 ProgramData 保护要求而跳过，已在隔离 Linux 中执行；真实 Windows DPAPI/ACL/Named Pipe 与系统浏览器流程仍须真机验收。外网 Steam、MMDB 样本、双机 NAT/Minecraft 及其他游戏本体未验收。本轮临时容器、卷、网络和预览已清理。

同日补充 Logto 参数说明及保存反馈后，管理台构建和 13 项测试通过，覆盖保存后重新打开表单恢复 issuer、client ID 和启用状态，以及 secret 不回显；管理台 `app.js` SHA256 为 `a730c238b83a6f079465ca3b8e09125814f4599ccd37d4cf6ce5d34ba4d2d992`。Go 控制包、架构和契约测试通过，本次未配置测试数据库，数据库用例跳过。已只读核对 `auth.nodelane.net` 的 discovery 元数据，尚未使用真实 App ID/Secret 完成登录，也未部署此次改动。

## LAN 改造检查（2026-09-11）

| 检查 | 实际结果与证据 |
|---|---|
| Windows Go | gofmt、`go vet ./...`、`go test -count=1 ./...` 通过，含真实 Nebula 进程内集成、LAN 授权/分片/去重/撤销及契约检查；未配置测试数据库。[vet](../.local/lan-windows-vet.log)、[测试](../.local/lan-windows-test.log) |
| Linux Go 与独立 PostgreSQL | vet、全量普通测试和 `go test -race -count=1 ./...` 通过；数据库用例实际执行，覆盖结构 4、拒绝旧库、MAC 绑定、完整端口区间、策略替换与旧接口移除。[普通测试](../.local/nodelane-test-20260911-111443-488854/test.log)、[race](../.local/nodelane-test-20260911-111443-488854/race.log) |
| 真实 Linux TAP/Nebula | 隔离双客户端经原生 relay，通过 TCP、IPv4/IPv6 UDP、32 KB 数据报、有限/子网广播、组播、游戏 UDP 4243 与诊断共存、动态策略收紧、跨房拒绝及离房/关房/踢人撤销；IPv4 底层 MTU 1280，TAP 1500。[记录](../.local/lan-final-verification.log)、[完整结果](../.local/nodelane-test-20260911-111443-488854/results.json) |
| 前端与桥接 | 桌面构建和 24 项测试、管理台构建和 11 项测试通过；Windows Rust 两项通过，已安装服务烟测跳过，验证旧端口操作被拒绝。[Rust](../.local/lan-rust-test.log) |
| 安装与打包检查 | Windows 临时目录/模拟 SCM 下安装 6 项、卸载 8 项通过；Python 打包 4 项通过，6 项 Linux 专用测试按平台跳过；未生成新完整包。[安装](../.local/lan-installer-test.log)、[卸载](../.local/lan-uninstall-test.log) |

进程内 Nebula 集成另用三成员内存 TAP 验证 1514 字节帧、广播/组播、游戏 UDP 4243 与诊断共存、策略收紧及到期关闭。IPX/LLC 模拟帧不代表旧游戏协议栈已验收。策略收紧先核对两端已应用新 revision，再验证旧端口和广播拒绝。

诊断曾出现探测超时：最终 12 次主动探测均回复，但两端统计窗口仍包含约 8.33% 和 7.69% 失败；原因与长期稳定性未确认，不能据一次成功声称零丢包。测试容器、私有卷、网络及本轮镜像已清理，未安装宿主驱动/服务、修改宿主防火墙或发布。

## 未验收与限制

- 最新 LAN 回归未运行完整原生 WebView 流程，仅检查脚本语法；历史 Linux socket/UID/WebView 验收不能代替当前 LAN 客户端验收。
- Windows 服务、UAC/SCM、安装 SID、Named Pipe/DPAPI/ACL、TAP 驱动签名与加载、完整安装/卸载及跨版本恢复，仍须真机验收。Linux 宿主 systemd、更新恢复、休眠、X11/Wayland、中文输入与无障碍，以及 ARM 真机也未完成。
- 不同 NAT 的双机直连/中继、真实公网 UDP/IPv6 底层、Minecraft Java 1.21.1 及其他游戏本体、接口选择、长期吞吐/丢包/广播压力仍未验收；Docker 网段、模拟帧和 ARM 仿真不能替代。步骤见 [人工验收](manual-v2-validation.md) 和 [游戏验收边界](game-network.md#能力与验收边界)。
- 管理台 DOM 测试不能代替真实浏览器的登录、Steam 导入、配置和展示全流程；生产页面与实际 1Panel/目标服务器部署仍未验收。
- 最新 Linux 回归未启用外网 Steam 烟测及 `NODELANE_TEST_GEOIP_DB`，两个 MMDB 样本测试跳过；历史 Windows Steam 下载和官方样本结果见下表。
- 已授权补丁未覆盖所有 Nebula 上游竞争，NodeLane 全量 race 通过不等于上游全仓库无竞争；详见 [补丁边界](nebula-race-review.md)。其他平台接入范围见 [客户端设计](client.md#后续平台的实际边界)。

## 复现

构建、普通测试、桌面检查和 Linux 数据库/race 命令见 [开发与构建](../README.md#开发与构建)，双客户端与部署回归见 [Docker 回归](../README.md#docker-双客户端回归)。数据库须使用独立 `NODELANE_TEST_DATABASE_URL`；MMDB 样本测试设置 `NODELANE_TEST_GEOIP_DB` 指向官方 `GeoIP2-City-Test.mmdb`。Steam 外网烟测设置 `NODELANE_TEST_STEAM=1` 后运行 `go test ./internal/control -run TestSteamLiveImport -v -count=1`；缺少配置时相应测试跳过。

## 必要历史证据

以下仅用于追溯；旧 TUN、端口登记和 Wintun 产物均不属于当前 LAN 验收。重复源码回归、已完成的临时排错过程及旧界面尺寸不保留。

| 日期与对象 | 当时的结果、适用范围及证据 |
|---|---|
| 2026-09-11 无边框 UI | 浏览器预览覆盖 1120×760、760×560、390×844 的布局与主要交互；24 项前端测试、2 项 Rust 测试和仅 GUI release 构建通过。`dist/desktop/nlroom-windows-amd64-frameless.exe` 附 SHA256、未签名；未验证原生拖动/托盘或完整 WebView，不是当前完整安装包 |
| 2026-09-11 Windows 安装器 | NSIS 完整包编译、安装 6 项、卸载 8 项及 Linux 容器全部 10 项打包检查通过；使用旧版 GUI/Go 产物，SCM 为模拟，不代表真实 UAC/驱动验收。[包摘要](../.local/installer-package-result.json)、[安装](../.local/installer-transaction-test.log)、[卸载](../.local/installer-uninstall-test.log)、[Linux](../.local/installer-linux-package-test.log) |
| 2026-09-11 桌面交付与 Linux WebView | 旧架构 deb 在容器内以普通用户经生产 socket 操作 root 后台，完成真实 HTTPS/数据库/Nebula 的房间、图片、邀请码复制、探测和退出 GUI 后继续联机；同时验证其他 UID 拒绝、同版本替换及 purge 保留身份。旧端口增删流程已删除；非 systemd 宿主验收。[WebView](../.local/nodelane-test-20260911-065931-51775f/desktop.log)、[网络回归](../.local/nodelane-test-20260911-063414-97385d/results.json) |
| 2026-09-11 旧 Windows 完整包 | payload 摘要及 WebView2/Wintun 签名通过，临时目录/模拟服务恢复 5 项通过；仅同版完整包重装，不证明不同身份格式可回退。[包检查](../.local/client-delivery-package-check.json)、[恢复](../.local/client-delivery-installer-test.log) |
| 2026-09-10 结构 3 与发布镜像 | 提交 `f32b411d03bd3fd062a3e216241610f983ff8d2c`：独立 PostgreSQL/race、旧 TUN 回归及镜像部署恢复通过；两份 0.2.0 镜像含 amd64/arm64，ARM 经仿真，固定摘要见 IMAGES.txt。Windows Steam 实际下载 Terraria 资料与两张图片通过。[数据库/网络](../.local/nodelane-test-20260910-205022-695472/results.json)、[部署](../.local/nodelane-deploy-test-c65f2f3f24d6/results.json)、[归档](../.local/fresh-release-check.log) |
| 2026-09-10 首版主机 UI | 9 项前端测试、2 项桥接测试和 Windows 仅 GUI 构建通过；浏览器预览不产生真实网络测量，未验证线上控制端。[视觉记录](../.local/console-ui/design-qa.md)、[当时网络回归](../.local/nodelane-test-20260910-230830-1cec92/results.json) |
| 2026-09-10 GeoIP/P2P | 官方 MMDB 损坏/月度回退、并发更新与恢复，以及无 relay 的真实双向 UDP P2P 通过；DB-IP 当月库下载校验成功。原结果 `.local/nodelane-test-20260910-173608-ddaace/results.json`，日志缺失 |
| 2026-09-10 页面与持久化部署 | 随机入口、默认路径拒绝、HTTPS 初始化、节点登记及重建后配置/CA/会话恢复通过。原结果 `.local/nodelane-deploy-test-b879e68c7b3a/results.json`，日志缺失 |
| 2026-09-09 至 09-10 扩展部署 | 双控制副本、登记恢复、UDP 配置确认、生命周期反馈、真实十分钟续签及断控到期停网通过。原结果 `.local/nodelane-deploy-test-41f5216cc2a6/results.json`，日志缺失 |
| 2026-09-10 多架构与 Wintun | 归档、架构、许可、执行权限、镜像摘要及 Wintun 签名检查通过；ARM 经仿真，未安装驱动或服务。原记录 `.local/v2-publish-20260910/`、`.local/v2-wintun-signatures.json`，日志缺失 |
