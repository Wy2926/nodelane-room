# 验证记录

仅在验证、排错或发布任务中读取。结果只对应当时源码或产物，跳过不算通过；新增记录注明提交或产物摘要、环境、结果和脱敏证据。历史记录未注明的提交不补推。`.local/` 不随源码保存，缺失日志仅保留原路径追溯。

## 当前源码与产物

当前为统一 Ethernet LAN、数据库结构 4、本机 IPC 2；最近产品验证为 2026-09-11 的 LAN 回归。下表保留原检查结果，后续改动不能据此自动判定通过；网络设计见 [游戏网络](game-network.md)。

| 对象 | 适用范围 |
|---|---|
| 当前 LAN 源码 | 单播、广播/组播、帧授权和分片已实现；组件和 Linux TAP 检查见下节，游戏本体仍未验收 |
| 当前完整安装包 | LAN 改造后尚未重新生成和验收；Windows TAP 驱动须独立准备 |
| 历史 0.2.0 安装包与镜像 | 使用旧数据面，不能与当前客户端混用；发布源码提交和镜像摘要见 [IMAGES.txt](../deploy/IMAGES.txt)，旧包/WebView 结果只列于历史证据 |

## 最近产品检查（2026-09-11）

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
