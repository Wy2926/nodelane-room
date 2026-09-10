# 验证记录

仅在验证、排错或发布任务中查阅。结果只对应当时的源码或产物，修改后须重验；源码、归档、发布镜像和真机分别判定，跳过不算通过。`.local/` 证据不随源码或发布包保存，缺失的历史日志仅保留原路径用于追溯。

## 最近源码检查（2026-09-10）

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

## 未验收与限制

- 最近一轮未配置 `NODELANE_TEST_GEOIP_DB`，两个依赖官方 MMDB 样本的测试跳过；此前样本测试通过的记录见下表。
- 通用单播查询、广播/组播 LAN 发现本轮仅完成 [方案](game-network.md)，尚未实现。已删除的 Minecraft 专用发现/代理测试不再计入当前通过范围；游戏内列表发现需按方案另行真机验收。
- Linux 隔离回归未启用外网 Steam 烟测；实际下载在 Windows 单独执行。管理台测试使用 DOM 环境，尚未完成真实浏览器从登录到导入、配置、展示的整段人工验收。
- 未完成生产管理页面端到端交互、实际 1Panel、Debian/Ubuntu systemd 安装与重复安装、跨版本更新回退及卸载。
- 未完成 Windows 服务安装/升级、Named Pipe 与 SID/ACL 真机联调、驱动、ARM 真机、跨服务器公网 UDP、双机不同 NAT、Minecraft Java 1.21.1 本体、长期稳定性及真实丢包验收。容器、协议模拟器和 ARM 仿真不能替代这些场景；步骤见 [人工及环境验收](manual-v2-validation.md)。
- GUI、Linux 普通用户控制 root 服务的安装与 UID 授权、移动端 VPN 接入尚未实现或验证，规划见 [客户端设计](client.md)。
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
