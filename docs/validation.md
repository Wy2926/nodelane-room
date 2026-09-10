# 验证记录

代码检查、发布产物和真机验收分别记录。历史发布结果只对应当时的产物；修改源码后须重新检查，不沿用“通过”。

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

管理页面交互与布局；真实 Debian/Ubuntu systemd 安装、重复安装、跨版本更新回退和卸载；实际 1Panel 与跨服务器公网 UDP；ARM 真机；Windows 服务、SID/ACL、驱动与升级；双机不同 NAT；Minecraft Java 1.21.1 本体；长期稳定性和真实丢包。操作清单见 [人工及环境验收](manual-v2-validation.md)。

握手补丁范围与其余上游竞争见 [Nebula 复核](nebula-race-review.md)。NodeLane 检查通过不能代表整个 Nebula 仓库检查通过。
