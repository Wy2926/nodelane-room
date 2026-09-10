# 文件索引

相对项目根目录，缩进表示层级；每行名称后为职责。列出维护文件，产物与缓存只标目录，不展开内容。

```text
.dockerignore 镜像构建排除项
.github/ CI 配置
  workflows/ 自动检查流程
    check.yml Windows 检查、Linux 数据库与 race 测试
.gitignore 版本管理排除项
.nvmrc Node.js LTS 开发与 CI 版本
.local/ 本地临时文件与验收日志
AGENTS.md 开发约束与索引维护规则
cmd/ 可执行程序入口
  nlroom-node/ Linux lighthouse/relay 命令
    main.go 登记、运行、诊断与原生安装管理
  nodelane/ Windows CLI 与后台服务
    main.go 玩家命令、输出与服务入口
  nodelane-server/ Linux 控制面命令
    main.go 控制实例启动、隐藏入口查询、初始化码与管理员恢复
deploy/ 部署模板与测试环境
  .env.example PostgreSQL 创建与 Caddy 域名配置示例
  .env.host.example 已有设施部署的可选绑定端口配置
  .env.node.example 独立节点配置示例
  Caddyfile HTTPS 与单控制实例反代
  compose.build.yaml 控制面本地构建覆盖项
  compose.host.yaml 复用已有数据库与反代的编排
  compose.network.yaml 接入已有 Docker 网络的覆盖项
  compose.node.build.yaml 节点本地构建覆盖项
  compose.node.yaml 独立节点容器编排
  compose.yaml PostgreSQL、单控制实例与 Caddy 编排
  Dockerfile.registry 从多架构发布包构建仓库镜像
  Dockerfile.release 从 Linux 发布包构建运行镜像
  IMAGES.txt 发布镜像版本与摘要
  nlroom-node.service 原生节点 systemd 单元
  node.sh Linux 原生节点安装脚本
  QUICKSTART.txt Compose 发布包部署速查
  test/ 隔离容器回归环境
    admin.py 回归用管理员认证与节点登记
    compose.yaml 数据库、控制面、中继与双客户端拓扑
    Dockerfile 测试程序与工具镜像
    entrypoint.sh 测试角色启动与容器内网络隔离
    peer.py TCP/UDP 与 Minecraft 公告测试辅助服务
dist/ 构建、安装包与镜像发布产物
Dockerfile 从源码构建控制面与节点镜像
docs/ 协议、部署与验收说明
  architecture.md 代码和接口分工、协议与安全边界
  deployment.md V2 部署与维护步骤
  files.md 文件层级与职责索引
  manual-v2-validation.md V2 人工及环境验收清单
  nebula-race-review.md 固定补丁原理与未处理的上游竞争
  openapi.yaml HTTP API 与数据结构契约
  validation.md 源码检查、历史发布证据与待验收项
go.mod 模块依赖与 Nebula 补丁锁定
go.sum 依赖校验和
internal/ 产品内部实现
  agent/ 玩家与节点后台状态协调
    health.go 存活与就绪检查
    local.go 本机服务接口与调用
    node.go 节点登记、配置同步、续签与状态
    node_doctor.go 节点诊断检查
    node_local.go 节点本机命令与脱敏日志缓冲
    runtime.go 玩家房间、证书、数据面与发现生命周期
    runtime_test.go 控制请求阻塞时的凭据到期测试
    telemetry.go 节点与玩家的短期监控采集和独立上报
    traffic_linux.go Linux 隧道网卡上传下载查询
    traffic_windows.go Windows IP Helper 隧道网卡计数查询
    traffic_other.go 其他平台的未知流量返回
    traffic_udp_linux.go Linux Nebula UDP 端口的被动包头计数
    traffic_udp_linux_test.go 真实 UDP 双向字节计数与去重测试
    traffic_udp_other.go 非 Linux 平台 UDP 观察器占位
  client/ 控制面 API 客户端
    api.go 设备身份、认证、幂等请求与 SSE 恢复
    api_test.go 控制地址校验测试
    enrollment.go 基础设施节点登记认证
  control/ 按调用方分文件的 HTTP API 与共享事务状态
    admin_audit.go 管理事件持久化与失败写操作审计
    admin_auth.go 管理员密码、登录与会话事务
    admin_events_http.go 管理台总览与事件流接口
    admin_http.go 管理员路由、认证包装与幂等写入
    admin_nodes_http.go 管理员节点配置、密钥、操作与编排下载
    admin_rooms_http.go 管理员房间详情与操作接口
    admin_session_http.go 管理员登录、会话与 Cookie/CSRF 校验
    admin_snapshot.go 管理台总览与房间详情的一致性快照
    admin_test.go 管理会话、CSRF、节点路由、幂等、审计与事件恢复测试
    admin_web.go 随机入口保护与持久化、管理页面和静态资源白名单
    admin_web_test.go 隐藏入口持久化、轮换、校验及默认关闭测试
    adminweb/ React 与 TypeScript 管理台
      dist/ Vite 编译产物，由 Go 嵌入
      node_modules/ npm 本地依赖缓存
      index.html React 页面入口
      package.json Node.js 版本、前端依赖与构建测试命令
      package-lock.json npm 依赖版本及完整性锁定
      tsconfig.json TypeScript 严格类型检查配置
      vite.config.ts Vite 同源资源构建与开发代理
      src/ 前端组件、数据处理与测试
        api.ts 管理员请求、CSRF 与认证失败处理
        auth.tsx 登录与创建或接入控制面的初始化表单
        auth.test.tsx 登录及接入已有控制面的交互测试
        components.tsx 表格、指标、表单及可访问对话框
        main.tsx 管理台导航、SSE、监控轮询与系统页面
        metrics.ts 新鲜度、速率、房间去重与出口观察计算
        metrics.test.ts 计数重置、断档、未知值与房间统计测试
        monitor.tsx 链路、出口与最近 60 秒趋势视图
        nodes.tsx 节点列表、配置及生命周期管理
        nodes.test.tsx 快照刷新期间保留配置版本的交互测试
        rooms.tsx 房间汇总、成员详情与授权操作
        style.css 管理台与移动端布局样式
        types.ts 前端 HTTP 与监控类型
    auth.go 设备挑战、会话与房间权限校验
    deployment.go 数据库配置校验、控制面创建与独立实例接入事务
    geoip.go 可并发切换的本地 MMDB 国家和省州查询
    geoip_update.go GeoIP 自动下载、月度检查、缓存校验及原子更新
    geoip_test.go 下载失败保留缓存、月度回退及官方样本并发查询测试
    http.go 公共 HTTP 入口、运维探针与响应处理
    http_test.go 路由白名单、OpenAPI 认证契约与会话边界测试
    install_http.go 原生节点安装资源下载白名单
    lease.go 玩家与节点短期隧道凭据签发及节点幂等校验
    node_http.go 节点登记、认证、同步与领证接口
    node_test.go 节点登记、身份隔离、配置与操作同步测试
    nodes.go 节点登记、配置、操作、撤销与同步
    player_events_http.go 玩家房间事件流与快照恢复
    player_http.go 玩家路由、认证、限速与幂等写入
    player_rooms_http.go 玩家房间查询、成员操作与领证接口
    player_test.go 房间、多副本、并发、授权与 SSE 测试
    rooms.go 房间、成员、心跳、邀请、端点与授权快照
    schema.sql V2 数据库表、共享配置、CA、索引与约束
    setup.go 无数据库页面入口、私有启动定位与控制实例恢复
    setup_test.go 初始化授权、上传 CA、事务回滚和多实例共享状态测试
    store.go 数据库初始化、幂等事务、地址分配、回收与限速
    store_test.go 数据库版本拒绝与数据保留测试
    test_helpers_test.go 独立测试 schema、HTTP 服务与玩家夹具
    telemetry.go 有界内存监控窗口、校验与过期清理
    telemetry_http.go 玩家和节点监控上报及管理员查询
    telemetry_test.go 监控授权、非持久化、过期和并发测试
  engine/ Nebula 数据面薄封装
    config.go 配置校验、生成与地址冲突入口
    engine.go 数据面生命周期、重载、撤销与真实路径查询
    nebula_integration_test.go 真实 Nebula 隔离、重载、撤销、中继与无中继 P2P 测试
    routes_linux.go Linux 路由冲突检查
    routes_other.go 其他平台路由检查占位
    routes_windows.go Windows 路由冲突检查
    telemetry.go 固定 Nebula hostmap 的实际路径、远端和隧道查询
  game/ 游戏端点与发现适配
    adapter.go 适配器接口与发现事件
    minecraft.go LAN 公告、授权发现、本机代理与组播回环
    minecraft_test.go 公告解析、授权、过期与代理清理测试
  model/ 共享协议类型
    model.go 设备、房间、凭据、端点与状态类型
    node.go 节点配置、登记、操作与同步类型
    telemetry.go 非持久化监控采样与窗口协议
  nodehost/ Linux 原生节点安装生命周期
    native.go 配置、发布包校验、更新、卸载与 systemd 操作
    native_test.go 解包安全与密钥输入测试
  pki/ Nebula 证书与隧道密钥
    pki.go CA 创建、PEM 校验、签发与隧道密钥生成
    pki_test.go 上传 CA 的地址池、组约束、有效期及多证书拒绝测试
  platform/ 平台权限、存储与本机通信
    diagnostics.go 系统、网卡与 TUN/Wintun 诊断
    platform_unix.go Unix 目录权限与本机套接字
    platform_windows.go DPAPI、ACL、Named Pipe 与 Windows 服务
    protection_windows_test.go Windows 私密数据与状态目录保护测试
    storage.go 设备身份与实例定位文件的保护和原子持久化
    sync_unix.go Unix 目录落盘同步
    sync_windows.go Windows 目录同步兼容处理
  probe/ 隧道内真实测量与发现消息
    probe.go 探测、RTT/丢包、授权广告与来源限速
    probe_test.go 探测与发现成员授权测试
README.md 产品说明、默认配置与操作入口
scripts/ 构建、安装与验证工具
  __pycache__/ Python 字节码缓存
  build-images.ps1 校验发布包并构建镜像，支持显式推送
  build.ps1 Windows/Linux 多架构发布包构建
  build.sh Linux 可执行文件构建
  check-release.py 归档校验和、内容、架构与权限检查
  Install.cmd Windows 双击安装入口
  install.ps1 Windows 安装、权限设置与服务就绪检查
  NodeLane.cmd 玩家命令行启动入口
  openapi/ API 契约生成工具
    main.go 更新协议类型、管理与节点接口及调用方分组
    main_test.go 契约重复生成稳定性与引用完整性检查
  package/ Linux 归档工具
    main.go 生成保留执行权限的 tar.gz
  package-compose.py 部署模板归档与校验和生成
  release/ 原生安装资源打包工具
    main.go 从发布包生成同源下载资源与清单
  setup.ps1 提权前捕获玩家 SID 并启动安装
  test-deploy.py 部署模板与发布镜像冒烟测试
  test-docker.py 隔离双客户端回归与可选 Go/race 检查
  uninstall.ps1 Windows 服务卸载与可选状态清理
THIRD_PARTY_NOTICES.md 第三方许可声明
```
