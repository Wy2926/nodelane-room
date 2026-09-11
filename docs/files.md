# 文件索引

## 按任务读取

先选任务行，再读对应实现、测试及文档章节。用 `rg -n '关键词' docs/files.md` 定位文件职责；涉及 HTTP 契约时只读取 OpenAPI 的目标路径及引用 schema。验收记录仅在验证、排错或发布时读取。

| 任务 | 实现与测试入口 | 文档章节 |
|---|---|---|
| 包依赖、进程分工 | `cmd/`、`scripts/architecture/` | [代码组织](architecture.md#代码组织) |
| 房间、成员、游戏配置 | `internal/control/` 的 `rooms`、`games`、`player_*`、`store` | [游戏规则](architecture.md#游戏目录与网络规则)、[事务与快照](architecture.md#事务与快照) |
| 管理台、初始化、节点 | `internal/control/admin*`、`setup*`、`node*`；`internal/agent/node*` | [控制实例](architecture.md#控制实例与配置)、[节点协议](architecture.md#v2-管理与节点协议) |
| LAN、授权、凭据到期 | `internal/lan/`、`internal/engine/`、`internal/agent/network.go`、`runtime*_test.go` | [数据面](architecture.md#数据面)、[游戏网络](game-network.md) |
| 桌面界面、本机 IPC | `desktop/src/`、`desktop/src-tauri/src/ipc/`、`internal/agent/local*`、`images.go` | [界面](client.md#主机界面与设计令牌)、[桌面架构](client.md#桌面架构) |
| 安装、权限、系统服务 | `internal/platform/`、`internal/nodehost/`、`scripts/desktop/`、安装脚本 | [本机权限](architecture.md#本机权限)、[客户端安装](../README.md#windows-客户端) |
| 监控、探测、GeoIP | `internal/probe/`、`internal/engine/telemetry.go`、`internal/agent/telemetry.go`、`traffic*`、`internal/control/telemetry*`、`geoip*` | [统计口径](architecture.md#监控)、[采集配置](deployment.md#监控与-ip-归属地) |
| 构建、部署、验收 | `scripts/`、`deploy/`、`.github/workflows/` | [构建](../README.md#开发与构建)、[部署](deployment.md)、[当前验收](validation.md#当前源码与产物) |

## 完整文件索引

路径相对项目根目录，两个空格表示一级缩进；维护文件每项一行。标注“（不展开）”的目录仅保留职责。Git 工作副本中的完整性由 `go test ./scripts/architecture` 检查；无 Git 元数据的源码副本跳过此项。

```text
.dockerignore 镜像构建排除项
.gitattributes Linux 脚本与生成契约的固定 LF 换行
.github/ CI 配置
  workflows/ 自动检查流程
    check.yml Windows 检查、Linux 数据库与 race 测试
.gitignore 版本管理排除项
.nvmrc Node.js LTS 开发与 CI 版本
.local/ 本地临时文件与验收日志（不展开）
AGENTS.md 开发约束与索引维护规则
cmd/ 可执行程序入口
  nlroom-node/ Linux lighthouse/relay 命令
    main.go 登记、运行、诊断与原生安装管理
  nlroom-cli/ Windows/Linux 开发与诊断 CLI
    main.go 玩家命令、机器可读输出与本机服务调用
  nlroom-service/ Windows/Linux 玩家网络后台
    main.go 后台生命周期与 Windows 服务管理入口
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
  nlroom-service.service Linux 玩家后台的 root 服务、目录保护与停止等待
  node.sh Linux 原生节点安装脚本
  QUICKSTART.txt Compose 发布包部署速查
  test/ 隔离容器回归环境
    admin.py 回归用管理员认证与节点登记
    compose.yaml 数据库、控制面、中继与双客户端拓扑
    Dockerfile 测试程序与工具镜像
    entrypoint.sh 测试角色启动与容器内网络隔离
    peer.py IPv4/IPv6 大包、广播/组播与 TCP/UDP 联机夹具
desktop/ Tauri 与 React 玩家客户端，主机风格独立于管理台
  index.html 正式桌面前端入口
  preview.html 仅开发使用的交互式界面预览入口
  icon.svg 既有 NodeLane 客户端品牌图标
  package.json 锁定前端运行依赖、图标与构建测试命令
  package-lock.json npm 精确依赖与完整性锁定
  tsconfig.json 客户端严格类型检查
  vite.config.ts 本地开发与 Vitest 配置
  public/ 随包客户端静态资源
    assets/ 原创主机氛围素材
      console-ambient.png 银蓝丝带背景与缺图回退
  src/ 客户端界面、样式与本机调用
    main.tsx 正式客户端挂载
    preview.tsx 独立开发预览夹具，模拟交互并明确无真实网络
    test-setup.ts DOM 测试环境、滚动与对话框模拟
    app/ 应用编排与系统导航
      App.tsx 本机状态、独立可达的系统页面与房间弹窗组合
      App.test.tsx 房间流程、诊断实测与脱敏、离线设置和更新占位测试
      Shell.tsx 统一顶部导航、玩家入口与无边框窗口控制
      navigation.ts 页面标识与中文标题
      Feedback.tsx 服务故障、忙碌与操作反馈
      use-actions.ts 操作互斥、复制与确认管理
    styles/ 午夜主题共享样式
      tokens.css 色彩、字阶、间距、尺寸、材质、焦点与动效令牌
      index.css 全客户端样式入口
      base.css 控件、排版、焦点与减少动态效果
      layout.css 主机舞台、顶部导航、窗口按钮与响应式布局
      forms.css 输入、模态弹窗、邀请码与端口样式
      feedback.css 错误、提示、空状态与开发预览标识
    native/ 真实 Tauri 本机桥接
      api.ts 玩家 IPC、错误文案、剪贴板与生命周期命令
      use-service.ts 不重叠状态轮询、退避与原生通知
      use-service.test.ts 版本一致性、并发刷新与服务恢复测试
    shared/ 共用数据与简单 UI
      model.ts 玩家、游戏、房间、本机请求与诊断返回类型
      time.ts 时间与状态新鲜度格式化
      ui/ 跨页面控件
        Empty.tsx 空状态与加载说明
        Modal.tsx 原生对话框、焦点和 Escape 行为
        PlayerAvatar.tsx 跨页面复用的玩家字母头像与氛围材质
        PortList.tsx 授权端口列表
    features/ 按玩家流程组织的页面
      catalog/ 服务端游戏目录与主机游戏选择
        GameLibrary.tsx 横向封面、键盘选择、搜索与建房入口
        Artwork.tsx 本机游戏图像展示与原创素材回退
        artwork-loader.ts 有界图像请求和缓存
        use-catalog.ts 目录、管理房间与陈旧状态加载
        catalog.css 游戏封面焦点和沉浸式信息舞台
      rooms/ 联机房间与成员
        RoomPage.tsx 欢迎主屏、房间选择、成员与连接布局
        RoomHero.tsx 当前游戏背景、房间状态及权限操作
        Members.tsx 玩家名片、虚拟 IP、实测链路与折叠管理菜单
        Connection.tsx LAN 就绪、游戏连接说明与后端只读配置
        use-room.ts 当前房间、管理快照与新鲜度判断
        rooms.css 欢迎舞台、房间卡片与连接布局
        members.css 玩家卡片、角色、菜单和实测链路样式
        dialogs/ 房间交互弹窗
          types.ts 弹窗状态类型
          RoomDialogs.tsx 房间弹窗调度与错误反馈
          CreateRoom.tsx 已选游戏的建房表单
          JoinRoom.tsx 邀请码入房表单
          Invitation.tsx 临时邀请码及复制
          Confirmation.tsx 权限操作和离房退出确认
      device/ 初始化与桌面偏好
        Setup.tsx 默认线上控制端、设备昵称与首次使用
        Settings.tsx 分类偏好、设备信息、更新界面占位与退出
        device.css 欢迎界面、分类设置与更新面板样式
      diagnostics/ 真实网络诊断
        Diagnostics.tsx 连接概览、系统检查、实测成员链路与脱敏摘要
        diagnostics.css 连接状态、系统检查与成员指标可视化样式
  src-tauri/ 原生窗口、托盘与受限本机 IPC 桥接
    Cargo.toml 原生依赖与程序信息
    Cargo.lock Rust 精确依赖锁定
    build.rs Tauri 构建入口
    tauri.conf.json 桌面窗口、资源与 CSP 边界
    capabilities/ 受限桌面命令能力
      main.json 主窗口允许调用的本机功能
    permissions/ 本机命令权限与生成配置（不展开）
    icons/ 原生窗口及安装图标（不展开）
    src/ 原生程序实现
      main.rs 窗口、托盘、通知与生命周期
      ipc/ 有界玩家服务桥接
        mod.rs 桥接命令入口
        protocol.rs 玩家请求校验与类型
        transport.rs Named Pipe 与 Unix socket 通信
        tests.rs 桥接请求与边界测试
dist/ 构建、安装包与镜像发布产物（不展开）
Dockerfile 从源码构建控制面与节点镜像
docs/ 协议、部署与验收说明
  architecture.md 包依赖、状态归属、权限、生命周期与监控口径
  client.md 当前桌面交互、GUI 决策、本机桥接与平台边界
  deployment.md V2 部署与维护步骤
  files.md 任务阅读导航与完整文件职责索引
  game-network.md Ethernet LAN 架构、MTU 评审、授权及游戏验收边界
  manual-v2-validation.md V2 人工及环境验收清单
  nebula-race-review.md 固定补丁原理与未处理的上游竞争
  openapi.yaml HTTP API 与数据结构契约
  validation.md 当前源码与产物验收、未验证项、复现入口及历史证据
go.mod 模块依赖与 Nebula 补丁锁定
go.sum 依赖校验和
internal/ 产品内部实现
  agent/ 玩家与节点后台状态协调
    health.go 存活与就绪检查
    images.go 受限控制端图片读取与本机图片响应
    local.go 本机 RPC 服务、玩家命令分派与后台启停
    local_test.go 玩家本机命令、协议版本与图片通道边界测试
    network.go 授权快照、凭据续签、Nebula 与探测生命周期
    node.go 节点登记、配置同步、续签与状态
    node_doctor.go 节点诊断检查
    node_local.go 节点本机命令与脱敏日志缓冲
    player.go 玩家初始化、游戏目录与房间操作
    runtime.go 共享运行状态、构造、持久化与后台主循环
    runtime_test.go 控制请求阻塞时的凭据到期测试
    status.go 玩家连接状态、实际探测与本机诊断
    telemetry.go 节点与玩家的短期监控采集和独立上报
    traffic_linux.go Linux 隧道网卡上传下载查询
    traffic_windows.go Windows IP Helper 隧道网卡计数查询
    traffic_other.go 其他平台的未知流量返回
    traffic_udp_linux.go Linux Nebula UDP 端口的被动包头计数
    traffic_udp_linux_test.go 真实 UDP 双向字节计数与去重测试
    traffic_udp_other.go 非 Linux 平台 UDP 观察器占位
  client/ 控制面 API 客户端
    api.go 控制面认证、幂等请求与 SSE 恢复
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
      dist/ Vite 编译产物，由 Go 嵌入（不展开）
      node_modules/ npm 本地依赖缓存（不展开）
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
        games.tsx Steam 导入、游戏草稿及统一 LAN/端口配置
        games.test.tsx 游戏版本保护、导入、完整端口范围与 LAN 策略测试
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
    game_import.go Steam 链接、资料及图片下载与来源和大小校验
    games.go 游戏目录、完整端口区间与 LAN 策略更新事务
    games_http.go 玩家游戏列表、图片和管理员导入配置接口
    games_test.go LAN 能力/MAC、规则并发、目录与图片授权测试
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
    player_desktop_test.go 桌面房间管理查询与离房房主权限测试
    player_events_http.go 玩家房间事件流与快照恢复
    player_http.go 玩家路由、认证、限速与幂等写入
    player_rooms_http.go 玩家房间查询、成员操作与领证接口
    player_test.go 房间、多副本、并发、授权与 SSE 测试
    rooms.go 房间、成员、心跳、邀请与 LAN 授权快照
    schema.sql 当前数据库表、共享配置、CA、索引与约束
    setup.go 无数据库页面入口、私有启动定位与控制实例恢复
    setup_test.go 初始化授权、上传 CA、事务回滚和多实例共享状态测试
    store.go 空库初始化与当前结构校验、幂等事务、地址分配、回收与限速
    store_test.go 数据库版本拒绝与数据保留测试
    test_helpers_test.go 独立测试 schema、HTTP 服务与玩家夹具
    telemetry.go 有界内存监控窗口、校验与过期清理
    telemetry_http.go 玩家和节点监控上报及管理员查询
    telemetry_test.go 监控授权、非持久化、过期和并发测试
  device/ 设备身份与持久化配置
    identity.go 设备身份生成、标识与控制地址校验
    identity_test.go 控制地址校验测试
  engine/ Nebula 生命周期与公开设备接入
    config.go 配置校验、生成与地址冲突入口
    engine.go 数据面生命周期、重载、撤销与真实路径查询
    lan.go 玩家 TAP DeviceFactory、网卡与内部探测状态
    lan_integration_test.go 真实 Nebula 三成员 Ethernet 分片、广播与撤销测试
    nebula_integration_test.go 真实 Nebula 隔离、重载、撤销、中继与无中继 P2P 测试
    routes_linux.go Linux 路由冲突检查
    routes_other.go 其他平台路由检查占位
    routes_windows.go Windows 路由冲突检查
    telemetry.go 固定 Nebula hostmap 的实际路径、远端和隧道查询
  lan/ 统一房间 Ethernet 数据面适配
    device.go 房间帧封装/重组、成员更新、去重与交付生命周期
    device_test.go 来源/端口授权、IPv4/IPv6 分片、发现、去重与撤销测试
    fragments.go IP 分片有界重组与端口校验前的重叠拒绝
    packet.go 标准 IP/Ethernet 报文解析、校验和与封装
    policy.go 成员 MAC/IP、发现、附加类型与反向连接授权
    probe.go 不占游戏端口的 Nebula 内部 PacketConn 与 TAP 读取
    tap.go TAP 句柄、地址与关闭清理
    tap_linux.go Linux 内核非持久 TAP 创建及 IPv4/IPv6 配置
    tap_windows.go 专用 TAP-Windows6 校验、异步 I/O 与地址配置
    tap_other.go 未支持平台的明确错误
  localapi/ 本机服务协议与调用客户端
    client.go 经 Named Pipe/Unix socket 调用本机 RPC
    client_unix_test.go Unix socket 请求、响应与错误兼容测试
    protocol.go 本机请求与节点日志响应类型
  model/ 共享协议类型
    game.go 游戏目录、LAN 策略/心跳、端口区间与管理类型
    lan.go LAN 版本、MAC、完整端口区间校验与 IPv6 地址推导
    model.go 设备、房间、凭据与 LAN 客户端状态类型
    node.go 节点配置、登记、操作与同步类型
    telemetry.go 非持久化监控采样与窗口协议
  nodehost/ Linux 原生节点安装生命周期
    native.go 配置、发布包校验、更新、卸载与 systemd 操作
    native_test.go 解包安全与密钥输入测试
  pki/ Nebula 证书与隧道密钥
    pki.go CA 创建、PEM 校验、签发与隧道密钥生成
    pki_test.go 上传 CA 的地址池、组约束、有效期及多证书拒绝测试
  platform/ 平台权限、存储与本机通信
    diagnostics.go 系统、LAN 网卡与 TUN/TAP 设备诊断
    local_linux.go Linux 桌面 root 服务、安装用户绑定与 socket 对端校验
    local_linux_test.go Linux 桌面真实 socket 与 UID 隔离测试
    local_other.go 非 Linux 平台的桌面 socket 接入占位
    platform_unix.go Unix 目录权限与本机套接字
    platform_windows.go DPAPI、ACL、Named Pipe 与 Windows 服务
    protection_windows_test.go Windows 私密数据与状态目录保护测试
    storage.go 设备身份与实例定位文件的保护和原子持久化
    sync_unix.go Unix 目录落盘同步
    sync_windows.go Windows 目录同步兼容处理
  probe/ 隧道内真实测量
    probe.go 经原生或 LAN PacketConn 的探测、RTT/丢包与来源限速
    probe_test.go 探测成员授权与统计过期测试
README.md 产品说明、默认配置与操作入口
scripts/ 构建、安装与验证工具
  __pycache__/ Python 字节码缓存（不展开）
  architecture/ 包依赖与文件索引检查
    boundaries_test.go 跨平台生产导入、数据库与数据面归属及客户端运行依赖边界
    files_test.go Git 维护文件与索引的遗漏、重复和失效检查
  build-images.ps1 校验发布包并构建镜像，支持显式推送
  build.ps1 Windows/Linux 多架构发布包构建
  build.sh Linux 可执行文件构建
  desktop/ Windows 与 Linux 桌面构建、交付与原生验收
    build.py 锁定版本的原生 GUI 构建、构建摘要与完整安装包入口
    branding.py 从既有品牌图标几何生成安装向导位图
    Dockerfile Ubuntu 22.04 桌面构建、Windows 交叉编译与 WebView 测试工具
    licenses.py 收集精确解析的桌面依赖声明与许可证
    package.py 校验版本和架构、分离运行文件与临时工具、生成 EXE/deb 与校验和
    windows.nsi 保留玩家账户的品牌安装、更新恢复与独立卸载向导
    linux-setup.sh 绑定 Linux 玩家 UID、启动后台与检查本机就绪
    linux-postinst.sh deb 安装或更新后重载并检查已绑定后台
    linux-prerm.sh deb 更新或移除前停止并等待旧后台退出
    linux-postrm.sh deb 移除后重载，保留身份和用户绑定
    test_package.py 版本、二进制架构、内容摘要与 Debian 生命周期测试
    test-windows.ps1 临时目录中的 GUI 文件替换、停止失败、登记失败和版本恢复测试
    test-uninstall.ps1 模拟 SCM 下的身份保留、显式清除、停止失败及卸载边界测试
    test_live.py 为已打包 Linux 客户端创建和清理隔离联机验收环境
    test-linux.sh 仅容器内的 deb 安装、真实 UID 隔离、替换及卸载验收
    test_webview.py 通过真实 WebView 和本机服务操作玩家完整联机流程
  check-release.py 归档校验和、内容、架构与权限检查
  Install.cmd Windows 双击安装入口
  install.ps1 Windows 完整包校验、用户绑定、安装更新与失败回滚
  NodeLaneRoom.cmd 玩家开发命令行启动入口
  openapi/ API 契约生成工具
    main.go 更新协议类型、管理与节点接口及调用方分组
    main_test.go 契约重复生成稳定性与引用完整性检查
  package/ Linux 归档工具
    main.go 生成保留执行权限的 tar.gz
  package-compose.py 部署模板归档与校验和生成
  release/ 原生安装资源打包工具
    main.go 从发布包生成同源下载资源与清单
  setup.ps1 提权前捕获玩家 SID 并启动安装或显式回滚
  test-deploy.py 部署模板与发布镜像冒烟测试
  test-docker.py 隔离 TAP/中继/低 MTU 回归与独立数据库 Go/race 检查
  uninstall.ps1 Windows 停止等待、受保护目录卸载与可选状态清理
THIRD_PARTY_NOTICES.md 第三方许可声明
```
