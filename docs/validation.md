# 验证记录

仅在验证、排错或发布任务中查阅。结果只对应当时的源码或产物，修改后须重验；源码、归档、发布镜像和真机分别判定，跳过不算通过。`.local/` 证据不随源码或发布包保存，缺失的历史日志仅保留原路径用于追溯。

## Windows 安装与卸载规范化（2026-09-11）

本轮使用 NSIS 3 Modern UI 2 提供品牌安装、维护恢复与独立卸载向导；运行目录不再包含安装脚本和 WebView2 引导程序。Go 数据面、权限协议、数据库结构与依赖未变。

| 检查 | 实际结果 |
|---|---|
| NSIS 安装包 | 使用 `makensis -WX` 编译完整 Windows amd64 EXE，安装和卸载各 4 页，SHA256 与校验文件一致；GUI/Go 二进制沿用同版产物。安装工具仅嵌入临时 engine，生成的原生卸载器随安装落盘。[构建](../.local/installer-build.log)、[产物摘要](../.local/installer-package-result.json) |
| Windows 安装事务 | 临时目录与模拟 SCM 下，GUI 包替换、停止失败、启动失败、就绪失败、显式恢复及系统登记失败恢复共 6 项通过；包含登记版本恢复。[记录](../.local/installer-transaction-test.log) |
| Windows 卸载 | 模拟 SCM 下，保留身份、显式清除、绑定玩家启动项清理、停止失败、残留进程、服务删除失败、不安全备份与异配服务共 8 项通过；已接入 CI。[记录](../.local/installer-uninstall-test.log) |
| 打包与 Linux 生命周期 | Windows 4 项打包检查通过，6 项 Debian 测试按平台跳过；Linux 容器运行全部 10 项通过，含运行文件与临时工具隔离。[Windows](../.local/installer-package-test.log)、[Linux](../.local/installer-linux-package-test.log) |
| Go 回归 | gofmt、Windows `go vet ./...` 和 `go test -count=1 ./...` 通过，含真实 Nebula 进程内集成测试；Windows 未配置测试数据库，数据库用例跳过。本轮无 Go 并发或控制状态改动，未重跑 Linux 数据库/race 回归。[vet](../.local/installer-go-vet.log)、[测试](../.local/installer-go-test.log) |

Windows UAC/SCM/Wintun 的全新安装、跨版本更新恢复和真实卸载仍须真机验收，ARM64 未验收；原生卸载窗口的自动预览被界面工具策略阻止，不能以编译和模拟用例代替完整界面验收。当前仍为未签名测试包，通过完整安装包手动更新，无在线更新源。此前跨平台联机证据见下一节。

## 桌面连接与交付检查（2026-09-11）

本轮沿真实返回格式修复了邀请码换新弹窗，补齐初始化前的设置/诊断、服务版本检查及离线时的表单保护，并交付 Windows EXE 和 Linux deb 完整包的安装、手动更新与恢复流程。Go 数据面、协议、数据库结构及 Nebula 版本未变。

| 范围 | 结果与证据 |
|---|---|
| 前端与原生构建 | Node.js 24 下 TypeScript/Vite 构建、15 项交互/轮询测试通过；Rust 1.95 Windows 和 Ubuntu 22.04 x64 release 构建通过。[前端测试](../.local/client-delivery-web-test.log)、[Windows 构建](../.local/client-delivery-windows-build.log)、[Linux 构建](../.local/client-delivery-linux-build.log) |
| 真实 Linux WebView | 在临时容器安装 deb，以普通用户经生产 Unix socket 操作 root Go 后台，连接独立 HTTPS/PostgreSQL/Nebula：初始化、游戏库、建房、端口增删、新邀请码及原生复制、离房后管理/关房、加入朋友房间、真实 ping、诊断和退出 GUI 后继续联机全部通过。[记录](../.local/nodelane-test-20260911-065931-51775f/desktop.log)、[房间截图](../.local/nodelane-test-20260911-065931-51775f/desktop/room.png) |
| Linux 包与权限 | 同一容器实际验证 deb 安装、同版本替换、purge 保留身份/UID，其他普通用户不能访问 socket；服务停止和重启由验收脚本显式执行。9 项打包/生命周期测试另覆盖停止失败、残留进程、就绪失败、禁用状态和 abort-upgrade。未将容器检查计为真实 systemd 验收 |
| Windows 包与恢复 | EXE 生成成功；解包后 617 项 payload SHA256 一致，WebView2/Wintun 分别通过 Microsoft/WireGuard 的 Authenticode 校验。临时目录与模拟服务下，升级、停止失败、启动失败、就绪失败和显式回滚 5 项事务测试通过。[完整包检查](../.local/client-delivery-package-check.json)、[恢复测试](../.local/client-delivery-installer-test.log) |
| Go 与真实网络回归 | gofmt、Windows `go vet ./...`、`go test -count=1 ./...` 通过；Windows 未配置测试数据库。Linux 独立 PostgreSQL 下 vet、全量普通/race 测试和真实 Nebula 双客户端回归通过，测试资源已清理。[完整结果](../.local/nodelane-test-20260911-063414-97385d/results.json) |

两平台产物位于 `dist/desktop/`，附 SHA256。EXE 为未签名测试包，deb 尚未进入签名仓库；无在线更新源。Windows/Linux Rust 各 2 项边界测试通过，独立的 `installed_service_roundtrip` 烟测均跳过；Linux 实际本机调用已由上述完整 WebView 验收覆盖，宿主未安装服务。ARM64、Windows UAC/SCM/Wintun 真机安装与跨版本升级、Linux systemd 宿主安装/跨版本恢复、休眠、Wayland 及双机 NAT/游戏本体验收仍待完成。容器替换只验证当前 0.2.0 完整包的重装和身份保留，不表示不同身份格式的版本可互相回退。

## 控制面与数据面基线（2026-09-10）

以下结果对应游戏目录、Steam 资料及图片导入、通用端口配置、Minecraft 专用发现移除和全新数据库结构版本 3。已同步 HTTP 契约；Nebula 固定版本未变。

| 范围 | 结果与证据 |
|---|---|
| Windows Go 1.27.0 | gofmt、vet、全量普通测试通过，含真实 Nebula、DPAPI/ACL、OpenAPI 和依赖检查；Windows 未配置测试数据库。[vet](../.local/fresh-windows-vet.log)、[测试](../.local/fresh-windows-test.log) |
| Linux Go 1.26.8、独立 PostgreSQL 18.6 | vet、全量普通测试和 `go test -race -count=1 ./...` 通过；空库初始化、拒绝旧结构、重开保留配置、跨副本读写、并发版本冲突、端口替换/停用、图片读取和幂等恢复的数据库用例实际执行。[结果及日志](../.local/nodelane-test-20260910-205022-695472/results.json) |
| 同次 Docker 双客户端回归 | 真实 TUN/Nebula 中继、CLI、默认拒绝、跨房隔离、离房/关房/踢人撤销、真实监控通过；游戏列表、固定端口自动登记及在线替换、自定义端口增删和客户端越权拒绝均验证了实际流量；本轮容器、卷、网络及测试镜像已清理 |
| 管理台 | Node.js 24.21.0 下 TypeScript/Vite 构建及 10 项前端测试通过，含导入失败保留链接、下载后编辑草稿、手动配置启用、并发编辑版本冲突保护和通用类型。[构建](../.local/fresh-frontend-build.log)、[测试](../.local/fresh-frontend-test.log) |
| Steam 实际下载 | Windows 显式运行 `NODELANE_TEST_STEAM=1` 的导入烟测通过；Terraria（105600）名称、简介、封面 62,177 字节和背景图 1,415,573 字节均下载并通过 JPEG/PNG 校验；无需 API Key |
| 发布包 | Windows/Linux amd64、arm64 四份归档及原生节点安装清单通过 SHA256、PE/ELF、执行权限与内容检查；未签名。[构建](../.local/fresh-release-build.log)、[归档检查](../.local/fresh-release-check.log) |
| 本地双架构镜像 | 控制面和节点的 amd64、arm64 程序可执行，镜像内程序 SHA256 与已校验发布包一致；ARM 通过仿真执行。[构建](../.local/fresh-images-build.log)、[平台校验](../.local/fresh-image-platforms.log) |
| 本地镜像部署 | `scripts/test-deploy.py --images` 通过全新初始化、HTTPS 登记、原生安装清单、实际 TUN、生命周期确认、节点身份及控制面配置/CA/会话重建恢复和永久撤销；本轮容器、卷和网络已清理。[结果及日志](../.local/nodelane-deploy-test-c65f2f3f24d6/results.json) |

上述程序来自提交 `f32b411d03bd3fd062a3e216241610f983ff8d2c`。两份 `0.2.0` 镜像已推送至 `docker.nodelane.net`；远端 OCI index 摘要与本地已测镜像一致，均含 amd64、arm64。标签、固定摘要和源码提交见 [IMAGES.txt](../deploy/IMAGES.txt)，远端核对日志：[控制面](../.local/fresh-control-remote.log)、[节点](../.local/fresh-node-remote.log)。

## 客户端主机主题（2026-09-10）

桌面 UI 采用 PS5 主屏幕与控制中心的视觉语言，未使用管理台设计。本次提交同时纳入首版 Tauri 客户端所依赖的公开状态、房间管理查询、受限图片 IPC、安装用户绑定和打包支持；Nebula 固定版本未变。

- Node.js 24.21.0 下 `npm run build`、9 项前端交互测试通过；覆盖陈旧目录拒绝建房、配置端口只读、离房失败不退出、建房防重复、游戏键盘选择与搜索、默认线上地址，以及成员菜单、确认返回焦点、真实零测量值和陈旧数据禁用操作。
- Windows 原生 `cargo build --locked --release --features custom-protocol` 与 2 项 Rust 桥接测试通过，生成 `dist/desktop/nlroom-windows-amd64-console.exe`（11,264,512 字节，未签名，仅 GUI）。未安装或启动宿主服务。证据：[前端构建](../.local/console-ui-build.log)、[交互测试](../.local/console-ui-test.log)、[原生构建](../.local/console-ui-native-build.log)、[桥接测试](../.local/console-ui-native-test.log)。
- 提交前 Windows `gofmt`、`go vet ./...`、`go test ./...` 通过；Windows 未配置测试数据库。证据：[vet](../.local/console-ui-go-vet.log)、[测试](../.local/console-ui-go-test.log)。
- `python scripts/test-docker.py --verify` 通过：Linux 独立 PostgreSQL 下 vet、全量普通测试、`go test -race -count=1 ./...`，以及真实 TUN/Nebula 双向通信、默认拒绝、端口替换与撤销、离房、关房、踢人与跨房隔离均通过；测试容器、卷、网络及本轮镜像已清理。[结果](../.local/nodelane-test-20260910-230830-1cec92/results.json)、[回归日志](../.local/console-ui-docker-test.log)。源码测试镜像带入桌面依赖清单以执行新增依赖边界检查。
- 在内置浏览器检查桌面主屏、横向游戏库、建房与邀请、房间详情、设置与诊断，以及 760×560 最小窗口和 390×844 窄屏首次使用。1280×720 下主操作完整可见；控制台无 error/warn。预览明确标注示例数据，未建立隧道或产生实测 RTT；不作为线上联机或原生 WebView 验收。[截图与视觉检查](../.local/console-ui/design-qa.md)。
- 线上地址已作为首次使用默认值，当前浏览器访问该站被阻止，本轮未验证线上控制端连接。既有身份不会被切换。
- 用户确认房间、成员名片、个人入口与设置的最终 UI；1120×760 默认窗口中成员卡片底边 670px，固定控制栏顶边 682px。菜单 Escape 返回入口、转让前确认和偏好开关已在开发预览操作；预览开关不修改宿主设置。

## 未验收与限制

- 最近一轮未配置 `NODELANE_TEST_GEOIP_DB`，两个依赖官方 MMDB 样本的测试跳过；此前样本测试通过的记录见下表。
- 通用单播查询、广播/组播 LAN 发现本轮仅完成 [方案](game-network.md)，尚未实现。已删除的 Minecraft 专用发现/代理测试不再计入当前通过范围；游戏内列表发现需按方案另行真机验收。
- Linux 隔离回归未启用外网 Steam 烟测；实际下载在 Windows 单独执行。管理台测试使用 DOM 环境，尚未完成真实浏览器从登录到导入、配置、展示的整段人工验收。
- 未完成生产管理页面端到端交互、实际 1Panel、Debian/Ubuntu systemd 安装与重复安装、跨版本更新回退及卸载。
- 未完成 Windows 服务安装/升级、Named Pipe 与 SID/ACL 真机联调、驱动、ARM 真机、跨服务器公网 UDP、双机不同 NAT、Minecraft Java 1.21.1 本体、长期稳定性及真实丢包验收。容器、协议模拟器和 ARM 仿真不能替代这些场景；步骤见 [人工及环境验收](manual-v2-validation.md)。
- Linux GUI、真实 socket/UID 与 WebView 的容器验收见本轮记录；Windows 原生服务、安装身份与系统 WebView，以及 Linux 宿主 systemd 的完整真机验收仍未完成。移动端 VPN 接入尚未验证，边界见 [客户端设计](client.md)。
- 已授权握手补丁及未处理的 Nebula 竞争见 [补丁边界](nebula-race-review.md)；NodeLane 全量 race 通过不代表 Nebula 全仓库无竞争。

## 复现

构建及普通测试命令见 [README](../README.md#开发与构建)，Linux 独立数据库/race、双客户端和部署回归见 [Docker 回归](../README.md#docker-双客户端回归)。直接运行数据库用例须设置独立 `NODELANE_TEST_DATABASE_URL`；样本测试须设置 `NODELANE_TEST_GEOIP_DB` 指向官方 `GeoIP2-City-Test.mmdb`。Steam 外网烟测需设置 `NODELANE_TEST_STEAM=1` 并运行 `go test ./internal/control -run TestSteamLiveImport -v -count=1`。缺少相应配置时用例跳过。

发布包先构建并运行 `scripts/check-release.py`；部署验证分别使用源码、`--root` 发布目录或 `--images --pull` 仓库镜像，`--host` 验证已有设施编排，`--extended` 等待真实十分钟证书周期。详细命令仍见 README。测试按唯一 Compose 项目清理，日志保存在 `.local/`。

## 必要历史证据

下列结果不作为最新源码验收结论。未列出的重复源码回归和已解决的临时排错过程不保留。

| 日期与对象 | 当时的结果与证据 |
|---|---|
| 2026-09-10 GeoIP/P2P | 官方 MMDB 的下载损坏回退、月度回退、并发更新/查询及恢复，以及无 relay 的真实双向 UDP P2P 通过；默认源 DB-IP 2026-09 City Lite 下载并校验成功。原结果 `.local/nodelane-test-20260910-173608-ddaace/results.json`，当前日志缺失 |
| 2026-09-10 页面与持久化部署 | 随机入口、资源白名单、根路径/旧入口拒绝、重建后入口保存、HTTPS 初始化、节点登记恢复及控制配置/会话恢复通过。原结果 `.local/nodelane-deploy-test-b879e68c7b3a/results.json`，当前日志缺失 |
| 2026-09-09 至 09-10 扩展部署 | 双控制副本、登记恢复、UDP 配置确认、生命周期反馈、真实十分钟续签及断控到期停网通过。原结果 `.local/nodelane-deploy-test-41f5216cc2a6/results.json`，当前日志缺失 |
| 2026-09-10 多架构与 Wintun | 归档内容、架构、许可、执行权限、镜像摘要及两架构 Wintun 签名检查通过；ARM 容器经仿真执行，未安装驱动或服务。原记录 `.local/v2-publish-20260910/`、`.local/v2-wintun-signatures.json`，当前日志缺失 |
