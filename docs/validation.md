# 验证记录

仅在验证、排错或发布任务中查阅。结果只对应当时的源码或产物，修改后须重验；源码、归档、发布镜像和真机分别判定，跳过不算通过。`.local/` 证据不随源码或发布包保存，缺失的历史日志仅保留原路径用于追溯。

## 最近源码检查（2026-09-10）

以下结果对应 Go 包边界整理后的源码，HTTP、身份存储格式和 Nebula 固定版本未变，未重建发布归档。

| 范围 | 结果与证据 |
|---|---|
| Windows Go 1.27.0 | gofmt、vet、全量普通测试通过，含真实 Nebula、DPAPI/ACL、OpenAPI 和依赖检查；Windows 未配置测试数据库。[vet](../.local/structure-windows-vet.log)、[测试](../.local/structure-windows-test.log) |
| Linux Go 1.26.8、独立 PostgreSQL 18.6 | vet、全量普通测试和全量 race 通过，数据库用例实际执行，含 Unix socket 请求/响应与错误兼容。[结果及日志](../.local/nodelane-test-20260910-193106-56e7bb/results.json) |
| 同次 Docker 双客户端回归 | 真实 TUN/Nebula 中继、CLI、默认拒绝、跨房隔离、离房/关房/踢人撤销、真实监控，以及模拟 Minecraft 发现、TTL 0 回环和已有 TCP 代理关闭均通过；本轮容器、卷、网络及测试镜像已清理 |
| 包边界与移动兼容 | Windows/Linux 依赖树符合边界，非宿主平台源码中的越界导入也被拒绝；运行时函数及本机协议移动比对通过。[依赖](../.local/structure-dependencies.json)、[拒绝](../.local/structure-guard-rejection.log)、[移动](../.local/structure-move-check.log) |

## 未验收与限制

- 最近一轮未配置 `NODELANE_TEST_GEOIP_DB`，两个依赖官方 MMDB 样本的测试跳过；此前样本测试通过的记录见下表。
- 未完成生产管理页面端到端交互、实际 1Panel、Debian/Ubuntu systemd 安装与重复安装、跨版本更新回退及卸载。
- 未完成 Windows 服务安装/升级、Named Pipe 与 SID/ACL 真机联调、驱动、ARM 真机、跨服务器公网 UDP、双机不同 NAT、Minecraft Java 1.21.1 本体、长期稳定性及真实丢包验收。容器、协议模拟器和 ARM 仿真不能替代这些场景；步骤见 [人工及环境验收](manual-v2-validation.md)。
- GUI、Linux 普通用户控制 root 服务的安装与 UID 授权、移动端 VPN 接入尚未实现或验证，规划见 [客户端设计](client.md)。
- 已授权握手补丁及未处理的 Nebula 竞争见 [补丁边界](nebula-race-review.md)；NodeLane 全量 race 通过不代表 Nebula 全仓库无竞争。

## 复现

构建及普通测试命令见 [README](../README.md#开发与构建)，Linux 独立数据库/race、双客户端和部署回归见 [Docker 回归](../README.md#docker-双客户端回归)。直接运行数据库用例须设置独立 `NODELANE_TEST_DATABASE_URL`；样本测试须设置 `NODELANE_TEST_GEOIP_DB` 指向官方 `GeoIP2-City-Test.mmdb`。缺少相应配置时用例跳过。

发布包先构建并运行 `scripts/check-release.py`；部署验证分别使用源码、`--root` 发布目录或 `--images --pull` 仓库镜像，`--host` 验证已有设施编排，`--extended` 等待真实十分钟证书周期。详细命令仍见 README。测试按唯一 Compose 项目清理，日志保存在 `.local/`。

## 必要历史证据

下列结果不作为最新源码验收结论。未列出的重复源码回归和已解决的临时排错过程不保留。

| 日期与对象 | 当时的结果与证据 |
|---|---|
| 2026-09-10 CLI/服务拆分后的本地归档 | Node.js 24.21.0、Windows Go 1.27.0 构建 Windows/Linux amd64、arm64 四份归档；模块、SHA256、PE/ELF、执行权限及安装内容检查通过。仅本地构建，未签名或发布。[构建](../.local/client-build-node24.log)、[归档检查](../.local/client-release-check.log) |
| 2026-09-10 GeoIP/P2P | 官方 MMDB 的下载损坏回退、月度回退、并发更新/查询及恢复，以及无 relay 的真实双向 UDP P2P 通过；默认源 DB-IP 2026-09 City Lite 下载并校验成功。原结果 `.local/nodelane-test-20260910-173608-ddaace/results.json`，当前日志缺失 |
| 2026-09-10 页面与持久化部署 | 随机入口、资源白名单、根路径/旧入口拒绝、重建后入口保存、HTTPS 初始化、节点登记恢复及控制配置/会话恢复通过。原结果 `.local/nodelane-deploy-test-b879e68c7b3a/results.json`，当前日志缺失 |
| 2026-09-09 至 09-10 扩展部署 | 双控制副本、登记恢复、UDP 配置确认、生命周期反馈、真实十分钟续签及断控到期停网通过。原结果 `.local/nodelane-deploy-test-41f5216cc2a6/results.json`，当前日志缺失 |
| 2026-09-10 发布镜像 | 实际拉取后的 14 项部署检查通过。原结果 `.local/nodelane-deploy-test-ff1744f6331a/results.json`，当前日志缺失 |
| 2026-09-10 多架构与 Wintun | 归档内容、架构、许可、执行权限、镜像摘要及两架构 Wintun 签名检查通过；ARM 容器经仿真执行，未安装驱动或服务。原记录 `.local/v2-publish-20260910/`、`.local/v2-wintun-signatures.json`，当前日志缺失 |

控制面与节点的 0.2.0 amd64/arm64 镜像于 2026-09-10 发布至 `docker.nodelane.net`，摘要见 [IMAGES.txt](../deploy/IMAGES.txt)。该历史发布早于后续页面初始化、管理台及客户端拆分等源码改动；验证新功能须重新构建。
