# V2 全新部署指南

版本 0.2.0。本指南重新部署控制端、节点和 Windows 客户端，不复用 V1 数据库、CA 或身份。迁移命令不会清空数据库；数据库版本不符时退出。先保留旧部署备份，再使用新的数据库、目录及 Compose 项目名。构建产物在 `dist/0.2.0/`；控制面与节点镜像已发布至 `docker.nodelane.net`，支持 `linux/amd64`、`linux/arm64`，实际摘要见 [IMAGES.txt](../deploy/IMAGES.txt)。

## 准备发布材料

直接部署可使用 `dist/compose/nodelane-room-compose-0.2.0.zip`，按下文填写配置并拉取指定版本镜像，无需本机构建。需要自行构建时，Windows/PowerShell 在源码根目录：

```powershell
./scripts/build.ps1
python scripts/check-release.py dist/0.2.0
python scripts/package-compose.py
./scripts/build-images.ps1
```

最后一条只在本机构建两个多架构镜像；只有显式 `-Push` 才推送。也可解压对应架构 Linux 发布包，在包目录用 `docker build --target control -t docker.nodelane.net/nodelane-room-control:0.2.0 .` 与 `--target node` 构建两个镜像。发布包中的 Dockerfile 直接使用已编译程序，不需要 Go 源码。

`releases/` 包含 node.sh、manifest.json、SHA256SUMS、amd64/arm64 原生节点包。它们随控制镜像复制到 `/opt/nodelane/releases`，由 `/install/` 同源提供。裸程序控制部署使用 `--release-dir` 指定只读目录。更新控制镜像时同时更新静态包；不要把可写上传目录配置为 release-dir。SHA256 校验的信任根是控制端 HTTPS。

## 控制机：已有 PostgreSQL / 1Panel 反代

在专用新目录放 `compose.host.yaml` 和 `.env.host.example`，后者复制为 `.env`（0600）。填写：

- `DATABASE_URL`：新建的空数据库。容器连接宿主机用私网地址/`host.docker.internal`；同一 Docker 网络可直接使用数据库容器名。不要用容器内 localhost 访问宿主。
- `PUBLIC_URL`：管理员和节点使用的完整 HTTPS origin，例如 `https://room.example.com`，不能带路径。
- `CONTROL_BIND_IP`：反代能到达的宿主私网/网桥地址；反代在宿主机直接运行时可用 127.0.0.1。
- 新的 `CONTROL_PROJECT_NAME`、`CA_DIR`；默认游戏地址池 `10.203.0.0/16`，须避开 LAN/Docker/VPN 网络。

配置完成后，在该目录拉取 0.2.0 镜像并执行初始化：

```bash
sudo install -d -m 0700 -o 10001 -g 10001 secrets
docker compose -f compose.host.yaml config --quiet
docker compose -f compose.host.yaml pull
docker compose -f compose.host.yaml run --rm --no-deps ca-init
docker compose -f compose.host.yaml up -d --wait
docker compose -f compose.host.yaml exec control-a nodelane-server admin bootstrap
```

`migrate` Exited(0) 正常，失败会阻止副本启动。CA 初始化不覆盖已有文件。CA 私钥只挂控制副本，权限目录 0700、文件 0600，属主 10001:10001；绝不复制给节点。

反代配置 HTTPS 域名与两后端 `http://CONTROL_BIND_IP:18080`、`:18081`，健康检查 `/readyz`，无需粘性会话。传递 Cookie、Origin、X-CSRF-Token、Authorization、Idempotency-Key、Last-Event-ID，关闭响应缓存和 SSE 缓冲，流超时至少 300 秒；公网拒绝 `/metrics`。保留原始 Origin，不能用后端 HTTP 地址替换。不要将控制 API 的未加密端口暴露公网。

1Panel 已有容器网络可添加 `compose.network.yaml`，或在 YAML 顶层设置 default 外部网络。数据库、反代必须同网；反代可改为 `control-a:8080`/`control-b:8080`，同时删除宿主 ports。加入网络不会自动修改 DATABASE_URL。

打开 `PUBLIC_URL/admin`，选择首次部署初始化，填写刚生成的 10 分钟一次性码并设置 12–128 字节密码，然后登录。密码遗失：

```bash
docker compose -f compose.host.yaml exec control-a nodelane-server admin reset-password
```

命令隐藏读取新密码，成功后所有管理会话失效。初始化码和密钥不应保存到工单、日志或 shell 历史。

## 控制机：同时部署 PostgreSQL 与 Caddy

使用独立 `compose.yaml`、Caddyfile、`.env.example`。填写 ROOM_DOMAIN、PUBLIC_URL、随机 URL 安全的 POSTGRES_PASSWORD，选择新项目名及不冲突地址池。DNS 指向控制机，反代所需 TCP 80/443 由管理员配置。步骤：

```bash
umask 077
cp .env.example .env
# 编辑 .env 后：
docker compose pull
mkdir -p secrets
chmod 700 secrets
CA_INIT_UID="$(id -u)" CA_INIT_GID="$(id -g)" docker compose run --rm --no-deps ca-init
sudo chown -R 10001:10001 secrets
docker compose up -d --wait
docker compose exec control-a nodelane-server admin bootstrap
```

不要与 compose.host.yaml 合并。不要执行整个 setup profile 的 up。数据库/CA 备份必须成套保留；不要用 down -v 处理错误。

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

CA 到期前 30/7/1 天查看管理台提醒，备份数据库和 CA、安排维护窗口，在隔离环境验证新的信任部署后重新登记节点/客户端。V2 不提供无感 CA 轮换，不能直接替换文件绕过数据库 CA 指纹和终端固定指纹。HTTPS 域名证书继续由反代维护，与 Nebula CA 分开。

完整发布前还需 [人工及环境验收](manual-v2-validation.md)。
