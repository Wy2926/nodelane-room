# 验证记录

代码检查、发布产物和真机验收分别记录。历史发布结果只对应当时的产物；修改源码后须重新检查，不沿用“通过”。

## 2026-09-10 隐藏管理入口、自动 GeoIP 与 P2P 核验

管理页首次启动生成 128 位随机入口并保护持久化，可用 `--admin-path` / `NODELANE_ADMIN_PATH` 配置、本机 `admin path` 查询；根路径及旧 `/admin` 返回 404，页面资源跟随入口。GeoIP 默认后台下载 DB-IP City Lite，每日检查月度版本、失败保留旧库并重试；完整 Compose 为控制容器增加出站网络。

- `gofmt`、Windows `go vet ./...`、`go test -count=1 ./...`、前端 6 项交互测试及 TypeScript/Vite 构建通过；Windows 未配置测试数据库。Compose 配置、Python 编译及 OpenAPI 生成稳定性通过。
- `python scripts/test-docker.py --verify` 通过：独立 Linux PostgreSQL 的 vet、全量普通测试与全量 race；真实 TUN/Nebula 中继、默认拒绝、跨房隔离、离房/关房撤销、监控及模拟 Minecraft 发现/代理撤销通过。[结果与日志](../.local/nodelane-test-20260910-173608-ddaace/results.json)。测试时将 `NODELANE_TEST_GEOIP_DB` 指向官方 `GeoIP2-City-Test.mmdb`，下载损坏保留缓存、月度回退、并发更新/查询及缓存恢复用例实际执行。
- 实际调用默认自动下载流程，成功下载并校验 DB-IP 2026-09 City Lite，原子缓存 127,339,927 字节 MMDB；文件在 `.local/geoip-download-check/cache/`，不进入发布产物。
- `python scripts/test-deploy.py --host` 通过：实际控制容器中的随机入口、JS/CSS、根路径及旧入口 404、无跳转、重建后入口持久化，以及 HTTPS 初始化、节点登记/恢复和控制配置/会话恢复。[部署结果](../.local/nodelane-deploy-test-b879e68c7b3a/results.json)。两轮临时容器、卷、网络和测试镜像均已清理。
- 生产 `punchy.punch/respond` 已开启；禁用直连只存在于中继测试的 lighthouse 地址拒绝列表和 Docker `BLOCK_SUBNET` 隔离。新增 `TestNebulaP2PWithoutRelay` 禁用测试客户端 relay，验证双向真实 UDP 直连、实际远端地址与未登记端口拒绝；Windows、Linux 普通与 race 均通过。

未发布镜像或安装包，未安装系统服务或修改宿主防火墙。未执行 Windows 服务/驱动真机验收、双机公网 NAT、实际 Minecraft Java 1.21.1 本体或生产浏览器端到端验收。容器模拟网络和游戏协议测试不替代这些验收。

## 2026-09-10 React 管理台与短期网络监控

管理台迁移到 React、TypeScript、Vite；Node.js 24.21.0 用于开发与构建，生产页面由 Go 同源提供。节点、房间和成员监控每 5 秒采样，内存保留 60 秒，15 秒无上报标记过期；连接类型和出口来自实际 Nebula 隧道，RTT/丢包来自真实探测。GeoIP 使用运维配置的本地 MMDB。

- `gofmt`、`go mod verify`、Windows `go vet ./...` 和 `go test -count=1 ./...` 通过。Windows 未配置测试数据库；数据库用例在下面的 Linux 独立 PostgreSQL 中执行。另以官方 MaxMind 测试 MMDB 验证公网国家/省州、IPv6 映射地址及私网未知值。
- `npm test` 的 3 个文件、6 项测试及 TypeScript/Vite 生产构建通过，覆盖初始化交互、节点配置版本保留、计数重置、采样断档与房间双向统计。OpenAPI 重新生成与稳定性检查通过。
- `python scripts/test-docker.py --verify` 全部通过：Linux vet、独立 PostgreSQL 普通测试及 `go test -race -count=1 ./...`；真实 TUN/Nebula 中继、默认拒绝、跨房隔离、离房/关房、模拟 Minecraft 发现与代理撤销均通过。管理接口收到两名成员和节点连续至少 30 秒的真实路径、RTT/丢包、成员流量、中继转发流量及节点观察到的成员 IP:端口。[最终结果及日志](../.local/nodelane-test-20260910-170916-b2b085/results.json)。
- 浏览器使用明确标注的合成数据检查桌面与 390px 窄屏、房间/成员详情、趋势图、出口与地区显示；页面无横向溢出，控制台无错误。该预览不代表生产浏览器端到端验收。
- 初次容器构建遇到依赖下载 EOF；后续监控验收发现 UDP 出站漏计和等待脚本重复登录触发限流，均修复后重跑通过。临时容器、卷、网络、测试镜像与页面预览已清理。

未发布镜像或安装包。未执行 Windows 网卡计数与服务/驱动真机验收、真实 systemd 权限升级、双机公网 NAT 或 Minecraft Java 1.21.1 本体验收；GeoIP 生产数据库须另行配置。

## 2026-09-10 单实例部署与页面初始化

控制模板改为每次一个实例；多个独立实例可验证已有管理员后接入同一数据库。页面填写数据库、公网地址、地址池、节点仓库，生成或上传 CA；共享配置和 CA 事务落库，本实例只持久化受保护的数据库定位信息。删除旧环境配置、CA 文件加载及 migrate/ca-init 前置任务，无旧初始化兼容。

- 最终 `gofmt`、Windows `go vet ./...`、`go test -count=1 ./...` 通过；Windows 未设置测试数据库，数据库及 Linux 控制实例私有目录测试在独立 PostgreSQL 中执行。
- 最终 Linux vet、普通测试及 `go test -race -count=1 ./...` 通过，包含多实例共享 CA/会话、初始化竞争、事务回滚、单实例数据库绑定冲突，以及上传 CA 的匹配、地址池、动态房间组和有效期检查。[最终源码结果](../.local/nodelane-test-20260910-162227-cc7111/results.json)。
- `python scripts/test-docker.py --verify` 的真实 TUN、Nebula relay、端口默认拒绝、跨房隔离、离房/关房与模拟 Minecraft 发现、代理撤销通过。[容器回归结果](../.local/nodelane-test-20260910-161838-54b09d/results.json)。
- `python scripts/test-deploy.py --host` 通过：模板单实例启动、页面数据库/CA 初始化、HTTPS、节点登记、控制及节点容器重建恢复、配置与生命周期操作。[部署结果](../.local/nodelane-deploy-test-b4040c534a64/results.json)。首次容器回归遇到测试脚本 CRLF 启动失败，改为 LF 后重跑通过。
- OpenAPI 重复生成与认证契约、JS 语法和 Python 编译检查通过。浏览器静态预览验证创建/接入/CA 上传模式、字段禁用及布局；HTTP 上传与初始化事务由 Go 测试验证。文件索引与部署文档同步。

本轮临时容器、卷、网络、测试镜像与页面预览均已清理。未发布镜像或安装包，未连接生产数据库；未执行生产浏览器端到端上传、实际 1Panel、Windows 服务/驱动、双机公网 NAT 与 Minecraft Java 1.21.1 本体验收。

## 2026-09-10 HTTP 具名处理函数整理

玩家、节点和管理员路由直接绑定具名处理函数，移除 HTTP 层按 URL 二次分发；房间、管理会话、节点管理和事件流按职责拆分。心跳、管理员身份事务与节点领证校验归入业务文件，保留原有认证、限速、请求摘要、幂等和审计行为，HTTP 契约未变。

- `gofmt`、Windows `go vet ./...` 与 `go test -count=1 ./...` 通过；Windows 未配置数据库，数据库用例由下述 Linux 独立库执行。
- `python scripts/test-docker.py --verify` 通过：Linux vet、独立 PostgreSQL 普通测试及全量 `go test -race -count=1 ./...`；真实 Nebula、双客户端 TUN、中继、端口默认拒绝、跨房隔离、离房/关房及模拟 Minecraft 代理撤销检查通过。[结果及日志](../.local/nodelane-test-20260910-151955-2a049b/results.json)。本轮容器、卷、网络与测试镜像已清理。
- 新增 OpenAPI 认证边界、管理员节点路由、写操作 Origin/CSRF、失败审计、密码变更撤销会话及跨接口幂等冲突回归；OpenAPI 稳定性与引用检查通过。文件索引和文档链接已同步核验。

未发布产品安装包或镜像，未执行 Windows 服务/驱动、双机公网 NAT 与 Minecraft Java 1.21.1 本体验收。

## 2026-09-10 接口与结构整理

管理、玩家和节点路由按职责分文件；管理页面资源独立存放。限制房间动作和公开静态文件，管理员房间详情使用独立事务快照。OpenAPI 同步收窄响应并按调用方分组。

- `gofmt`、Windows `go vet ./...` 与 `go test -count=1 ./...` 通过；Windows 未配置数据库，数据库用例由下述 Linux 独立库执行。
- `python scripts/test-docker.py --verify` 通过：Linux vet、普通测试、全量 race，以及双客户端真实 TUN、中继、端口默认拒绝、跨房隔离、离房/关房和模拟 Minecraft 代理撤销。包含本轮新增的身份隔离、管理员房间详情、HEAD、未知路由与静态资源边界检查；[结果及日志](../.local/nodelane-test-20260910-145718-e4d36c/results.json)。本次容器、卷、网络与测试镜像已清理。
- OpenAPI 重复生成稳定、内部引用完整；文件索引与本地文档链接检查通过。

未重建或发布产品安装包/镜像，未运行下文列出的页面、系统安装及真机验收。

## 0.2.0 发布记录（2026-09-09 至 09-10）

环境为 Windows Go、Docker Linux、独立 PostgreSQL 18.6，固定 Nebula v1.11.1 与授权握手补丁。控制面与节点的 amd64/arm64 镜像于 9 月 10 日发布到 `docker.nodelane.net`；摘要见 [IMAGES.txt](../deploy/IMAGES.txt)。

| 范围 | 当时的结果与保留证据 |
|---|---|
| Windows 代码与数据库 | gofmt、vet、普通测试通过，独立数据库用例实际执行；[构建检查日志](../.local/v2-final-check-build.log) |
| Linux 代码与数据库 | vet、普通测试及全量 race 通过，含真实 Nebula 集成；[race 日志](../.local/v2-final-linux-race.log) |
| 双玩家真实 TUN | 默认拒绝、跨房隔离、TCP/UDP、中继、离房/关房、模拟 Minecraft 公告、TTL 0 回环及代理撤销通过；[结果](../.local/nodelane-test-20260909-155357-d75010/results.json) |
| 扩展部署 | 双控制副本、登记恢复、UDP 配置确认、生命周期反馈、真实十分钟证书续签及断控到期停网通过；[结果](../.local/nodelane-deploy-test-41f5216cc2a6/results.json) |
| 远端发布镜像 | 实际拉取后的 14 项部署检查通过；[结果](../.local/nodelane-deploy-test-ff1744f6331a/results.json) |
| 多架构产物 | 校验和、架构、执行权限、许可和安装资源检查通过；[镜像核对](../.local/v2-publish-20260910/image-check.json)、[远端摘要](../.local/v2-publish-20260910/registry-check.json) |
| Wintun | 两架构签名检查通过，未安装驱动或服务；[记录](../.local/v2-wintun-signatures.json) |

上述 `.local/` 证据仅在执行检查的工作区保留，不进入发布包。ARM 容器命令经仿真执行，Minecraft 使用协议模拟器。历史源码检查不表示管理页面、ARM 或游戏真机验收通过。

## 复现

```powershell
nvm use 24.21.0
npm --prefix internal/control/adminweb ci
npm --prefix internal/control/adminweb test
npm --prefix internal/control/adminweb run build
go vet ./...
go test -count=1 ./...
python scripts/test-docker.py --verify
./scripts/build.ps1
python scripts/check-release.py dist/0.2.0
python scripts/test-deploy.py --root dist/0.2.0/nodelane-room-0.2.0-linux-amd64
```

`test-docker.py --verify` 在隔离 Linux 容器与 PostgreSQL 中执行 vet、普通测试及 `go test -race -count=1 ./...`。直接运行数据库用例需设置独立 `NODELANE_TEST_DATABASE_URL`，缺少时跳过，跳过不算通过。

`test-deploy.py --images --pull --host` 检查仓库镜像；`--extended` 额外等待真实十分钟证书周期。源码、发布包和仓库镜像应分别验证。测试不发布宿主端口，按独立 Compose 项目清理容器、卷及网络，日志保存在 `.local/`。

## 未验收与限制

管理页面生产端到端交互；真实 Debian/Ubuntu systemd 安装、重复安装、跨版本更新回退和卸载；实际 1Panel 与跨服务器公网 UDP；ARM 真机；Windows 服务、SID/ACL、驱动与升级；双机不同 NAT；Minecraft Java 1.21.1 本体；长期稳定性和真实丢包。操作清单见 [人工及环境验收](manual-v2-validation.md)。

握手补丁范围与其余上游竞争见 [Nebula 复核](nebula-race-review.md)。NodeLane 检查通过不能代表整个 Nebula 仓库检查通过。
