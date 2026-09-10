# 控制面部署指南

当前源码提供单实例部署和页面初始化。历史 0.2.0 发布镜像不包含本次改动；验证或部署此流程须先从当前源码构建控制镜像。无旧环境变量、CA 文件或旧初始化接口兼容；已有旧控制面不自动转换或清库。

## 构建材料

源码镜像构建增加 Node.js 24 LTS 阶段，使用锁定的 npm 依赖编译 React 管理台；运行镜像仍为 Go。直接 `go build` 前执行 `npm --prefix internal/control/adminweb ci` 和 `npm --prefix internal/control/adminweb run build`。

### 监控与 IP 归属地

监控完整性要求相关节点、玩家和浏览器使用同一个控制实例，重启后重新积累。采样窗口与统计口径见 [架构说明](architecture.md#监控)。

Linux 节点上传/下载包含 Nebula UDP 中继转发，需被动观察包头。新 `compose.node.yaml` 和管理台下载的 YAML 已包含 `cap_add: [NET_ADMIN, NET_RAW]`；旧容器请更新后重建。原生节点 systemd 单元增加 `CAP_NET_RAW` 与 `RestrictAddressFamilies` 的 `AF_PACKET`，由运维更新单元并重启。缺少采集权限时业务仍运行，监控流量显示 `—`；不应以宿主机所有网卡流量代替。

国家/省州默认自动下载 [DB-IP City Lite](https://db-ip.com/db/download/ip-to-city-lite)，无需账号或手动下载。库按月发布，服务启动后异步拉取，每 24 小时检查更新；失败保留旧库并每小时重试，首次遇当月文件尚未发布则尝试上月。缓存为实例状态卷内 `geoip.mmdb`，重启直接使用；下载经 HTTPS、超时、大小限制及 MMDB 校验后原子替换。页面保留 DB-IP 署名。首次下载前、记录缺失、私网或没有实际观察到的出口均显示未知。

默认源为 `https://download.db-ip.com/free/dbip-city-lite-{month}.mmdb.gz`，`{month}` 替换为 `YYYY-MM`。可用 `--geoip-url` / `NODELANE_GEOIP_URL` 指定 HTTPS `.mmdb.gz` 镜像源。完整 Compose 为控制容器增加独立出站网络；已有 Docker 网络也须允许 DNS 和出站 HTTPS。查询始终在本地，成员 IP 不发给第三方。

已有自管数据库时，可显式加 `--geoip-db /opt/nodelane/geoip/GeoIP.mmdb` 停用自动下载。Compose 可合并只读挂载与环境变量，保留原状态卷：

```yaml
environment:
  NODELANE_GEOIP_DB: /opt/nodelane/geoip/GeoIP.mmdb
volumes:
  - ./geoip/GeoIP.mmdb:/opt/nodelane/geoip/GeoIP.mmdb:ro
```

手动覆盖文件必须可由镜像 UID 10001 读取，且遵守对应数据库许可；指定无效文件会拒绝启动。Country 库只含国家，省州需 City 库。系统页面显示当前是否已加载可查询的库。

源码目录本地构建后启动已有设施模板：

```bash
docker compose -f deploy/compose.host.yaml -f deploy/compose.build.yaml build
docker compose -f deploy/compose.host.yaml up -d --wait
docker compose -f deploy/compose.host.yaml exec control nodelane-server admin path
docker compose -f deploy/compose.host.yaml exec control nodelane-server admin bootstrap
```

Linux 发布包也可用相同构建覆盖文件，从包内程序构建镜像。完整产物由 `scripts/build.ps1`、`scripts/check-release.py`、`scripts/package-compose.py` 和 `scripts/build-images.ps1` 构建及校验；推送须显式 `-Push`。

`releases/` 随控制镜像复制到 `/opt/nodelane/releases`，同源提供 node.sh、manifest.json、校验和及 amd64/arm64 原生节点包。裸程序使用 `--release-dir` 指定只读目录；不要把上传目录配置为安装资源目录。

## 已有 PostgreSQL / 1Panel 反代

`compose.host.yaml` 仅启动一个 `control`，默认访问地址为 `127.0.0.1:18080`，无需创建业务配置 `.env`。反代直接在宿主运行时可使用默认地址；反代容器可通过 `CONTROL_BIND_IP` 设置能访问的宿主私网地址，或与控制容器加入同一 Docker 网络。

1Panel 使用已有 `1panel-network` 时，叠加 `compose.network.yaml`，或将其中的 `networks` 段加入单文件编排。数据库与控制容器必须互通。反代同网时可直接连接 `control:8080` 并删除宿主 `ports`；同一网络部署多个独立项目时应给反代使用唯一容器名或网络别名，避免多个 `control` 别名混用。

游戏资料导入要求控制实例能访问 `store.steampowered.com:443` 和 `*.steamstatic.com:443`；导入最长 45 秒，反代写请求超时需留出余量。资料和下载图片保存在 PostgreSQL，随数据库备份和恢复；仅图库图片为公开静态资源，管理接口仍需会话及 CSRF。配置步骤见 [README 游戏管理](../README.md#游戏管理)。

反代须配置 HTTPS，保留原始 Host、Origin、Cookie、Authorization、X-CSRF-Token、Idempotency-Key、Last-Event-ID；关闭响应缓存和 SSE 缓冲，流超时至少 300 秒，公网拒绝 `/metrics`。代理存活检查使用 `/healthz`，使未初始化页面也可访问；`/readyz` 只在数据库配置和 CA 加载成功后返回 200。不要公开未加密控制端口。

## 页面初始化

先在控制实例终端执行 `nodelane-server admin path`，用公网 HTTPS 域名加输出路径打开初始化页面。入口首次启动用 128 位随机值生成，保存在受保护的 `admin-path.bin`，保留状态卷即可在重建后继续使用。根路径、旧 `/admin` 及旧静态资源返回 404，无默认跳转；入口不出现在服务日志、公开 API 或快照中。管理员 API 仍在 `/v2/admin/*`，继续校验密码、Cookie、Origin 与 CSRF。

可配置 `serve --admin-path /your-private-admin-entry` 或环境变量 `NODELANE_ADMIN_PATH`，格式为 `/` 加 16–128 位 ASCII 字母、数字、下划线或短横线。重建/重启后生效并持久化，旧入口随之失效；移除配置会继续使用已保存值。多实例可各用随机入口；同域负载均衡时需显式配置相同入口或按实例分配域名。使用自定义状态目录的查询命令也要传 `--state-dir`。

执行 `nodelane-server admin bootstrap` 获取 10 分钟初始化码；只保存码的哈希，不记录到服务日志。页面选择“创建控制面”，填写：

| 配置 | 保存及用途 |
|---|---|
| 管理员账号、密码 | 创建唯一管理员，密码保存 Argon2id 哈希；密码 12–128 字节 |
| PostgreSQL 连接串 | 页面填写，创建或验证当前 schema（版本 3）；初始连接串保存在数据库，实例私有目录自动保存启动定位副本 |
| 公网地址 | 例如 `room.nodelane.net`，自动补全 HTTPS；须与当前页面 origin 一致，用于管理授权及节点安装 |
| 游戏地址池 | 默认 `10.203.0.0/16`，支持规范 IPv4 /16 至 /28；须避开 LAN、Docker、VPN |
| 节点镜像仓库 | 默认 `docker.nodelane.net`，用于管理台生成的节点 Compose |
| CA | 直接生成一年有效 CA，或上传 `ca.crt` 和 `ca.key`；校验匹配、自签名、有效期和地址池，拒绝限制动态房间组的 CA |

连接串使用 `postgres://用户:密码@主机:5432/数据库?sslmode=disable` 形式，不包含 shell 引号。密码中的特殊字符须按 URL 编码。数据库主机是控制容器可解析的名称或地址；同一 Docker 网络可使用 PostgreSQL 容器名和内部端口，不能把容器内 localhost 当作宿主。`sslmode` 遵循数据库策略。

初始化仅接受空 schema 或未使用且地址池一致的当前 schema（版本 3）；不迁移旧结构，不补建旧库的表；已有管理员、节点或旧 CA 的控制面不会被覆盖。数据库 schema、管理员、地址池、公网地址、仓库和 CA 在一个事务中提交。失败不会留下部分管理员或部分 CA；若本地连接已落盘而事务未提交，使用同一数据库重试。

CA 证书和私钥均保存在 PostgreSQL，控制实例按需加载，节点只能取得 CA 公钥证书。CA 和数据库密码不会出现在管理快照、事件或生成的节点 YAML 中。地址池和 CA 初始化后固定，本次不提供运行中更换。

`CA_DIR` 已删除。`CONTROL_PROJECT_NAME` 使用 Compose 固定默认名 `nodelane-room`，需要同机多个项目时用 `docker compose -p <实例名>`。`NODELANE_REGISTRY` 不再是控制镜像环境参数，控制镜像从 YAML 中的明确地址拉取；页面的镜像仓库只控制后续节点 YAML。它们不能由尚未启动的控制服务决定。

## 接入同一控制面的其他实例

每次独立部署仍只有一个控制服务，各自使用独立 `control-state` 卷，不共享本地目录：

1. 新实例启动后，在该实例终端获取初始化码。
2. 访问该实例的 HTTPS 管理页面，选择“接入已有控制面”。
3. 填写同一数据库的连接串及已有管理员账号密码。
4. 服务验证管理员后自动加载数据库中的公网地址、地址池、仓库和 CA；不会创建新管理员或覆盖配置。
5. 将新实例加入统一公网地址的反代后端。正式登录和节点访问仍使用数据库中保存的统一公网地址。

共享状态、会话、地址、撤销、事件、幂等和事务锁都在 PostgreSQL，不要求粘性会话。不同实例可以使用各自可达的数据库地址，但必须指向同一数据库和 schema。

## 本地持久化与备份

唯一不可只保存在数据库中的配置是“如何连接数据库”。每个实例自动写入 `/var/lib/nodelane-control/database.bin`，通过 `control-state` 持久化；无需事前填写环境变量或手工创建文件。Linux 目录 0700、文件 0600，控制容器使用 UID/GID 10001，根文件系统只读。该目录只包含本实例的数据库定位信息与初始化码哈希，CA 不落本地文件。

重建容器须保留该卷。数据库暂时不可用时，服务保持存活并等待连接，业务返回未就绪；不会回退环境配置或新建控制面。卷丢失可用新实例的初始化码和现有管理员重新接入原数据库。

备份 PostgreSQL（包含 CA 私钥及共享配置）并保护各实例私有卷；不要用 `down -v` 排错。连接信息、数据库备份和 CA 都应按私密数据保护。密码遗失时在已配置实例终端执行：

```bash
docker compose -f compose.host.yaml exec control nodelane-server admin reset-password
```

命令隐藏读取新密码，成功后所有实例的管理员会话失效。

## 同时部署 PostgreSQL 与 Caddy

使用独立 `compose.yaml`、Caddyfile 和 `.env.example`，每套仍只有一个控制实例。只需提前填写 `ROOM_DOMAIN` 和用于创建 PostgreSQL 的随机 `POSTGRES_PASSWORD`；DNS、80/443 和反代 HTTPS 属于基础设施配置。业务配置仍在页面填写。

```bash
# 在 deploy 目录；从当前源码构建
umask 077
cp .env.example .env
# 编辑 ROOM_DOMAIN 与 POSTGRES_PASSWORD
docker compose -f compose.yaml -f compose.build.yaml build
docker compose up -d --wait
docker compose exec control nodelane-server admin bootstrap
```

页面数据库主机填 `db:5432`，用户与数据库名均为 `nodelane`，密码使用上述同一值。不再执行 `migrate`、`ca-init` 或准备 secrets 目录。不与已有设施模板合并。增加接入实例时使用已有设施模板，避免另建数据库。

## 节点预创建和临时接入密钥

在管理台“节点 → 添加节点”填写名称、区域、真实公网域名/IP 和 UDP 端口，默认 4242，lighthouse/relay 默认同时启用。至少需要一个可达 lighthouse。公网入口是节点服务器地址，HTTPS 反代不能代替此 UDP 入口。

创建后生成临时接入密钥，明文仅展示一次，30 分钟有效，一次登记后失效；签发即代表授权，无二次审批。生成响应丢失则重新生成。修改待登记配置会撤销旧密钥。不要把同一节点状态目录复制到多台正在运行的服务器；替换机器在管理台执行“替换服务器”，新机器使用全新目录和新密钥。

## 路径一：curl 原生安装

支持 Debian 12/13、Ubuntu 22.04/24.04，amd64/arm64，systemd。需要 root 交互终端、TUN，以及 curl/python3/tar/coreutils/iproute2/util-linux 等基础工具。宿主安装了 Docker 不影响此脚本；在容器内或不支持的系统上会拒绝。

```bash
curl -fsSL https://room.example.com/install/node.sh | bash -s -- --server https://room.example.com
```

非默认池附加 `--network <控制台地址池>`，固定版本附加 `--version 0.2.0`。脚本定义完整流程后才执行，检查目标、网段、系统和权限，下载明确版本并校验 SHA256 后安装。不会自动修改防火墙、Docker 或 TUN 设备权限。

程序 `/usr/local/bin/nlroom-node`；配置 `/etc/nlroom-node/config.json`；状态 `/var/lib/nlroom-node`（0700，身份文件 0600）；systemd 服务使用专用 `nlroom-node` 账号和 CAP_NET_ADMIN。安装后从 `/dev/tty` 隐藏读取密钥。失败保留配置和身份，修复后执行 `nlroom-node enroll`；已登记时验证并恢复，不重新消耗密钥。

重复 curl 保留已有身份及配置并启动服务。不同控制端或不明身份冲突会报错，不能覆盖。程序升级用 `nlroom-node update 0.2.0`：只接受控制端当前清单公布的明确版本，校验归档与程序版本，原子替换，等待新进程健康，失败回退程序；不回退身份。异常断电留下 update.lock 时先检查没有更新进程，再人工处理锁文件。卸载用 `nlroom-node uninstall`，默认保留配置、身份和服务账号；弃用服务器还应在控制台永久撤销身份。

## 路径二：Compose / 1Panel

管理台可下载已填写控制 URL、端口和 0.2.0 镜像的节点 YAML，不含密钥。或使用 `compose.node.yaml` 与 `.env.node.example`，填写 NODE_CONTROL_URL、NODE_PORT、NODE_STATE_DIR；固定 NODELANE_VERSION=0.2.0。先由宿主准备持久状态目录（0700、属主 root:root），例如 `sudo install -d -m 0700 -o 0 -g 0 ./state`；管理台生成 YAML 时使用其中的绝对路径。确保 `/dev/net/tun` 可用。节点容器仅额外授予 NET_ADMIN，映射需要的 UDP 端口。

```bash
docker compose -f compose.node.yaml up -d --wait
docker compose -f compose.node.yaml exec node nlroom-node enroll
docker compose -f compose.node.yaml exec node nlroom-node status
```

1Panel 中直接在节点容器的终端执行 `nlroom-node enroll`，隐藏粘贴密钥。未登记进程保持运行，`healthz=200`，`readyz=503`，因此 `up --wait` 不表示隧道已经上线。成功登记后查看 readyz 和 status。

镜像运行前台 `nlroom-node run`；容器内不安装 systemd，不管理宿主 Docker。容器执行 service/update/uninstall 给出 Compose/1Panel 操作提示。升级更改镜像版本并重建，保留同一状态挂载，不能删除身份目录。

## 日常命令和配置

| 命令 | 用途 |
|---|---|
| enroll | 临时密钥登记，或恢复已有身份 |
| status --json / --watch | 控制连接、引擎、证书及配置状态 |
| doctor --json | TUN、NET_ADMIN、DNS、HTTPS、时钟、地址/路由及声明端口检查 |
| config | 期望/已应用配置与版本 |
| config apply 版本 | 确认需要本机参与的配置 |
| logs / logs --follow | 当前进程最近 1000 条有界脱敏日志 |
| restart | 只重启 Nebula，保留管理进程 |
| service start/stop/restart | 原生 systemd 服务管理 |
| update 版本 / uninstall | 原生更新/保留身份卸载 |
| version | 产品、协议和固定 Nebula 版本 |

这些命令通过 0600 Unix Socket 操作已有进程，需 root 或专用服务账号。原生日志进入 journal，容器日志进入标准输出/错误，可用 Compose logs 查看跨进程历史。

名称/区域/备注自动同步；角色和游戏端口规则需安全重启数据面。公网地址变化使旧外部探测记录失效。UDP 端口变更保留原入口，显示待应用：原生先安排宿主放行新 UDP，然后 `config apply <版本>`；容器先更新 YAML 的端口、启动参数及 NLROOM_MAPPED_PORT，重建，再执行 apply。配置版本已经变化时拒绝旧 apply。

停止分配保留身份和续签，仅取消新连接候选；停用撤销当前证书并停止数据面，保留受限管理；恢复允许重新领证；永久撤销和替换旧身份不能再认证。离线操作显示等待，不能提前算成功。

## 诊断、证书和 CA 维护

控制连接只证明 HTTPS；45 秒未心跳显示中断。引擎状态来自本机进程。公网入口的已验证状态必须来自其他已登记节点的实际直连与探测，保留来源、时间和路径；中继成功不能证明目标入口 UDP 直达。首次只有一个节点时尚未验证是正常状态，不阻塞客户端发现。

证书最多 10 分钟，剩余约 7 分钟开始带少量抖动自动续签；证书到期检查独立于网络请求，到期停止数据面，控制恢复后重新领证。无需管理员手动刷新日常证书。撤销无法使失联机器立即知道，但失联不能延长当前证书。

CA 到期前 30/7/1 天查看管理台提醒，备份含 CA 的数据库、安排维护窗口，在隔离环境验证新的信任部署后重新登记节点/客户端。V2 不提供无感 CA 轮换，不能直接修改数据库 CA 绕过终端固定指纹。HTTPS 域名证书继续由反代维护，与 Nebula CA 分开。

完整发布前还需 [人工及环境验收](manual-v2-validation.md)。
