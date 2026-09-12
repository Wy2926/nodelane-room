# 文件索引

## 按任务读取

先选任务行，再读对应实现、测试及文档章节。用 `rg -n '关键词' docs/files.md` 定位文件职责；涉及 HTTP 契约时只读取 OpenAPI 的目标路径及引用 schema。验收记录仅在验证、排错或发布时读取。

| 任务 | 实现与测试入口 | 文档章节 |
|---|---|---|
| 包依赖、进程分工 | `cmd/`、`scripts/architecture/` | [代码组织](architecture.md#代码组织) |
| 用户、访客、OIDC | `internal/control/users*`、`oidc*`、`internal/agent/account.go`、`internal/client/account.go`、`internal/model/user.go` | [用户身份](architecture.md#用户身份)、[账号登录配置](deployment.md#账号登录配置) |
| 房间、成员、游戏配置 | `internal/control/` 的 `rooms`、`games`、`player_*`、`store` | [游戏规则](architecture.md#游戏目录与网络规则)、[事务与快照](architecture.md#事务与快照) |
| 官网、管理导航与设置 | `internal/control/site_web*`、`siteweb/`、`adminweb/src/navigation*`、`settings*`、`update-*` | [官网入口](deployment.md#官网与管理入口)、[管理操作](deployment.md#管理操作划分) |
| 管理台、初始化、节点 | `internal/control/admin*`、`setup*`、`node*`；`internal/agent/node*` | [控制实例](architecture.md#控制实例与配置)、[节点协议](architecture.md#v2-管理与节点协议) |
| LAN、授权、凭据到期 | `internal/lan/`、`internal/engine/`、`internal/agent/network.go`、`runtime*_test.go` | [数据面](architecture.md#数据面)、[游戏网络](game-network.md) |
| 桌面界面、本机 IPC | `desktop/src/`、`desktop/src-tauri/src/ipc/`、`internal/agent/local*`、`images.go` | [界面](client.md#界面与视觉规范)、[桌面架构](client.md#桌面架构) |
| 客户端更新、版本规则 | `internal/update/`、`internal/agent/updates.go`、`internal/control/updates*`、`scripts/updates/` | [更新与发布](updates.md) |
| 安装、权限、系统服务 | `internal/update/`、`internal/platform/`、`internal/nodehost/`、`scripts/desktop/` | [本机权限](architecture.md#本机权限)、[客户端安装](../README.md#windows-客户端) |
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
  nlroom-update/ 特权更新助手入口
    main.go 原生安装、启动 TAP 准备与受保护更新任务入口
  nlroom-node/ Linux lighthouse/relay 命令
    main.go 登记、运行、诊断与原生安装管理
  nlroom-cli/ Windows/Linux 开发与诊断 CLI
    main.go 玩家命令、机器可读输出与本机服务调用
    main_test.go 用户错误、系统不可达与结果未知的有限退出码测试
  nlroom-service/ Windows/Linux 玩家网络后台
    main.go 后台生命周期与 Windows 服务管理入口
  nodelane-server/ Linux 控制面命令
    main.go 控制实例启动、隐藏入口查询、初始化码与管理员恢复
deploy/ 部署模板与测试环境
  nlroom-update.timer Linux 开机未完成更新恢复
  nlroom-update.service 独立 root 更新单元与包管理器生命周期
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
  src/ 客户端界面、样式与本机调用
    assets/ 新界面静态素材
      brand-mark.png 从选定设计提取的珊瑚色品牌图形
      generic-room.png 暖白珊瑚色双手柄通用房间封面
    main.tsx 正式客户端挂载
    preview.tsx 中英文开发预览夹具与截图样例，模拟交互并明确无真实网络
    test-setup.ts DOM 测试环境与对话框模拟
    app/ 应用编排与系统导航
      App.tsx 本机状态、独立可达的系统页面与房间弹窗组合
      App.test.tsx 房间置顶、首次语言、诊断自动刷新与旧回复隔离及离线设置测试
      RenderBoundary.tsx 页面渲染故障隔离与恢复入口
      Shell.tsx 客户端版本顶栏、常规三段与全窗口引导布局、个人菜单和关闭窗口
      navigation.ts 页面标识与标题字典键
      Feedback.tsx 服务故障、忙碌与操作反馈
      OperationFeedback.tsx 页面与弹窗共用的接管、未决查询和执行进度
      Problem.tsx 业务错误、恢复入口与支持详情
      experience.ts 连接状态展示、操作可用性与恢复动作映射
      use-actions.ts 写操作互斥、账号接管、复制与确认管理
      use-operations.ts 未决收据查询、过期核对与异步回复隔离
      use-actions.test.ts 未决写互斥、接管子步骤与跨服务实例旧回复测试
    i18n/ 客户端语言选择与翻译
      index.ts 语言偏好持久化、订阅与字典插值
      LanguageSelection.tsx 全窗口首次语言选择与偏好保存
      i18n.test.ts 字典键、插值、日期与源码文案边界检查
      locales/ 按语言独立维护的界面及原生文案
        zh-CN.json 简体中文客户端字典
        en-US.json 英文客户端字典
    styles/ 暖白与珊瑚色共享样式
      tokens.css 颜色、字体、圆角与焦点变量
      index.css 全客户端样式入口
      base.css 控件、排版、焦点、隐藏滚动条与减少动态效果
      layout.css 窗口边缘与三段布局、房间列表、入房侧栏及系统页响应式样式
      forms.css 输入、模态弹窗、邀请码与端口样式
      feedback.css 错误、提示、空状态与开发预览标识
    native/ 真实 Tauri 本机桥接
      api.ts 玩家 IPC、默认控制端、错误文案、剪贴板与生命周期命令
      use-service.ts 不重叠状态轮询、退避与原生通知
      use-service.test.ts 版本一致性、并发刷新与服务恢复测试
      use-query.ts 目录、房间和账号共用的有界轮询与旧回复隔离
    shared/ 共用数据与简单 UI
      model.ts 玩家、游戏、房间、本机请求与诊断返回类型
      time.ts 时间与状态新鲜度格式化
      ui/ 跨页面控件
        Empty.tsx 空状态与加载说明
        Modal.tsx 原生对话框、焦点和 Escape 行为
        PlayerAvatar.tsx 以昵称首字符呈现玩家头像
        PortList.tsx 授权端口列表
    features/ 按玩家流程组织的页面
      catalog/ 服务端游戏目录与本机游戏图片
        Artwork.tsx 本地通用房间封面、本机游戏图片与缺图状态
        artwork-loader.ts 有界图像请求和缓存
        use-catalog.ts 目录、管理房间与陈旧状态加载
      rooms/ 联机房间与成员
        RoomPage.tsx 当前房间置顶列表、入房侧栏、满宽封面与详情导航
        RoomHero.tsx 封面下方的房间标题、主要动作与连接状态
        Members.tsx 对齐成员表、虚拟 IP、最近 30 秒实测与管理菜单
        Connection.tsx LAN 就绪、游戏连接说明与后端只读配置
        use-room.ts 当前房间、管理快照与新鲜度判断
        dialogs/ 房间交互弹窗
          types.ts 弹窗状态类型
          RoomDialogs.tsx 房间弹窗调度与错误反馈
          CreateRoom.tsx 固定通用房间、名称与服务端建房许可表单
          Invitation.tsx 临时邀请码及复制
          Confirmation.tsx 权限操作和离房退出确认
      device/ 初始化与桌面偏好
        Updates.tsx 版本页面、启动更新提示、跳过版本、下载进度与取消及自动安装
        Updates.test.tsx 启动提示、跳过持久化、下载取消与自动安装及离线展示测试
        Setup.tsx 简洁登录主入口、次级访客昵称与首次使用
        Account.tsx 账号资料、共用登录操作与授权设备管理
        use-account.ts 登录能力、事务与账号设备查询
        use-account.test.ts 身份切换与旧登录回复回归
        Settings.tsx 独立账号分类、桌面偏好、设备信息与本机更新入口
      diagnostics/ 真实网络诊断
        Diagnostics.tsx 连接与授权状态、进入页面自动执行本机系统检查
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
      main.rs 窗口、托盘、通知、语言同步与生命周期
      language.rs 复用前端语言字典的原生文案与语言白名单测试
      startup.rs 已安装 Windows GUI 的启动 TAP 检查、按需提权与失败提示
      ipc/ 有界玩家服务桥接
        mod.rs 桥接命令入口
        protocol.rs 玩家请求校验与类型
        transport.rs Named Pipe 与 Unix socket 通信
        tests.rs 桥接请求与边界测试
dist/ 构建、安装包与镜像发布产物（不展开）
Dockerfile 从源码构建控制面与节点镜像
docs/ 协议、部署与验收说明
  updates.md 更新信任、发布、多源、安装与强制规则操作
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
  update/ 签名更新验证、下载与特权安装引擎
    bundle.go Windows 程序载荷校验、逐文件覆盖与过期文件清理
    bundle_test.go 载荷路径、摘要、架构及安装中断后覆盖修复测试
    host_windows.go 原生 ACL、SCM、进程等待、卸载登记和开始菜单快捷方式
    host_windows_test.go 孤立网卡登记、只读 PnP、签名、子进程退出和原生快捷方式测试
    setup_windows.go 原生安装升级卸载、SID 绑定、UAC 和受限状态管道
    setup_other.go 非 Windows 平台跳过原生安装命令
    tap_windows.go 原生 PnP TAP 检查、签名验证、按需安装与按登记 GUID 清理
    trust_test.go 签名篡改、过期与元数据回退测试
    trust.go TUF 信任链、版本防回退与目标验证
    install_windows.go Windows 提权后临时运行器与完整安装包启动
    install_other.go 其他平台拒绝系统安装
    install_linux.go Linux root 更新、APT 锁与可信 deb 恢复
    install.go 持久安装任务、二次校验、健康检查与平台安装结果
    download_test.go HTTPS 恢复下载与错误响应验证
    download.go 多镜像断点续传与完整包校验
  agent/ 玩家与节点后台状态协调
    interaction.go 独立暂停与恢复网络、账号和邀请只读查询
    operations.go 受保护操作账本、同键重放与收据对账
    operations_test.go 原始字节持久化、状态非阻塞与暂停恢复测试
    updates_test.go 无房间版本上报、GUI 版本变化与更新同意及请求取消回归
    updates.go 更新轮询、缓存策略、下载调度与设备版本上报
    account.go 受保护的登录事务、本机账号切换与退出
    health.go 存活与就绪检查
    images.go 受限控制端图片读取与本机图片响应
    local.go 本机 RPC 服务、玩家命令分派与后台启停
    local_test.go 玩家本机命令、协议版本与图片通道边界测试
    network.go 授权快照、凭据续签、Nebula 与探测生命周期
    probe.go 独立于控制请求与界面的后台定时探测
    node.go 节点登记、配置同步、续签与状态
    node_doctor.go 节点诊断检查
    node_local.go 节点本机命令与脱敏日志缓冲
    player.go 玩家初始化、游戏目录与房间操作
    runtime.go 共享运行状态、构造、持久化与后台主循环
    runtime_test.go 控制阻塞时的到期停网、自动探测及账号注销恢复测试
    status.go 玩家连接状态、实际探测与本机诊断
    telemetry.go 节点与玩家的短期监控采集和独立上报
    traffic_linux.go Linux 隧道网卡上传下载查询
    traffic_windows.go Windows IP Helper 隧道网卡计数查询
    traffic_other.go 其他平台的未知流量返回
    traffic_udp_linux.go Linux Nebula UDP 端口的被动包头计数
    traffic_udp_linux_test.go 真实 UDP 双向字节计数与去重测试
    traffic_udp_other.go 非 Linux 平台 UDP 观察器占位
  client/ 控制面 API 客户端
    api_test.go 同键重试、精确授权终态与快照整体替换测试
    account.go OIDC 登录发起、设备证明领取与账号状态读取
    api.go 控制面认证、幂等请求与 SSE 恢复
    enrollment.go 基础设施节点登记认证
  control/ 公开官网、私有管理页面、按调用方分文件的 HTTP API 与共享事务状态
    site_web.go 与控制状态独立的中英官网模板渲染、语言路由及静态资源白名单
    site_web_test.go 中英官网页面切换、资源白名单、方法边界及管理入口隔离测试
    siteweb/ 与桌面风格一致的公开中英双语多页官网
      layout.html 共用页面骨架、双语导航、语言切换、页脚和元信息
      index.html 产品介绍、应用截图与主要入口
      product.html 房间、邀请、连接状态与中文桌面截图展示
      download.html 后台发布驱动的客户端版本选择、平台要求与安装指引
      help.html 入门步骤、联机排错与常见问题
      about.html 产品理念、开源项目与联系渠道
      privacy.html 账号、设备、房间及诊断数据的隐私说明
      terms.html 产品用途、使用条件与支持范围说明
      en/ 官网对应的英文页面内容
        index.html 英文产品介绍、应用截图与主要入口
        product.html 英文房间、邀请、连接状态与桌面截图展示
        download.html 英文客户端版本选择、平台要求与安装指引
        help.html 英文入门步骤、联机排错与常见问题
        about.html 英文产品理念、开源项目与联系渠道
        privacy.html 英文账号、设备、房间及诊断数据的隐私说明
        terms.html 英文产品用途、使用条件与支持范围说明
      assets/ 官网本地样式与应用截图
        site.css 暖白珊瑚色、多页共享与响应式官网样式
        downloads.js 官网双语版本加载、平台选择与按需下载地址获取
        brand-mark.png 复用的 NodeLane 应用品牌图形
        app-lobby.jpg 当前应用房间大厅截图
        app-lobby-en.jpg 当前应用英文房间大厅截图
        app-room.jpg 当前应用房间成员截图
        app-room-en.jpg 当前应用英文房间成员截图
        app-settings.jpg 当前应用账号设置截图
        app-settings-en.jpg 当前应用英文账号设置截图
    updates_test.go 强制撤销、共享修订、公开下载可用性与存储凭据保护回归
    updates_http.go 更新管理、包上传、官网下载、公开检查与版本上报的 HTTP 适配
    updates.go 签名发布、版本规则与强制授权撤销事务
    update_storage.go 加密凭据、更新源连接、包上传与副本校验提交
    update_snapshot.go 公开下载签名与来源核对、更新检查及管理更新总览查询
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
    admin_web_test.go 隐藏入口持久化、轮换、校验与官网隔离测试
    adminweb/ React 与 TypeScript 管理台
      dist/ Vite 编译产物，由 Go 嵌入（不展开）
      node_modules/ npm 本地依赖缓存（不展开）
      index.html React 页面入口
      preview.html 仅开发使用的管理台演示入口
      package.json Node.js 版本、前端依赖与构建测试命令
      package-lock.json npm 依赖版本及完整性锁定
      tsconfig.json TypeScript 严格类型检查配置
      vite.config.ts Vite 同源资源构建与开发代理
      src/ 前端组件、数据处理与测试
        updates.test.tsx 独立存储与发布页面、配置修订、凭据只写及陈旧响应回归
        updates.tsx 签名清单、安装包草稿、验证与发布操作
        update-data.ts 更新快照的加载、刷新与错误状态
        update-sources.tsx 独立更新存储源配置及连接验证
        update-policies.tsx 独立推荐和最低版本规则设置
        update-devices.tsx 设备版本分布、报告及安装结果查询
        api.ts 管理员请求、CSRF 与认证失败处理
        operations.tsx 管理员未决收据查询与过期核对
        auth.tsx 登录与创建或接入控制面的初始化表单
        auth.test.tsx 登录及接入已有控制面的交互测试
        components.tsx 表格、指标、表单及可访问对话框
        games.tsx Steam 导入、游戏草稿及统一 LAN/端口配置
        games.test.tsx 游戏版本保护、导入、完整端口范围与 LAN 策略测试
        main.tsx 管理台页面装配、SSE 与监控轮询
        main.test.tsx 管理分类导航、页面地址恢复与设置职责分离回归
        navigation.tsx 按操作职责分组的导航与地址 hash 页面定位
        settings.tsx 独立 OIDC 与管理员密码设置页面
        settings.test.tsx 账号登录设置、凭据清除与密码修改交互回归
        site-downloads.test.js 官网双语版本选择、下载失效与空状态交互回归
        preview.tsx 仅开发演示数据与管理台交互夹具
        metrics.ts 新鲜度、速率、房间去重与出口观察计算
        metrics.test.ts 计数重置、断档、未知值与房间统计测试
        monitor.tsx 链路、出口与最近 60 秒趋势视图
        nodes.tsx 节点列表、配置及生命周期管理
        nodes.test.tsx 快照刷新期间保留配置版本的交互测试
        rooms.tsx 房间汇总、成员详情与授权操作
        style.css 管理台与移动端布局样式
        types.ts 前端 HTTP 与监控类型
        users.tsx 用户分页、设备、会话和账号授权管理
        users.test.tsx 用户限制建房的操作原因与授权交互回归
    auth.go 显式访客登记、设备挑战、会话与房间权限校验
    users.go 用户查询、事务授权检查、设备与管理操作及连接撤销
    users_http.go 用户与设备路由、请求解码和 HTTP 响应
    users_test.go OIDC 完整流程、身份升级、冲突、跨设备封禁与撤销测试
    oidc.go OIDC 配置、登录状态、设备证明与身份绑定事务
    oidc_http.go OIDC 路由、浏览器 Cookie、回调与确认页面
    operations.go 事务收据、拒绝保存点、截止时间与重放权限
    operations_test.go 跨副本收据、回滚、秘密可见性及契约拒绝测试
    deployment.go 数据库配置校验、控制面创建与独立实例接入事务
    game_import.go Steam 链接、资料及图片下载与来源和大小校验
    games.go 游戏目录、完整端口区间与 LAN 策略更新事务
    games_http.go 玩家游戏列表、图片和管理员导入配置接口
    games_test.go LAN 能力/MAC、规则并发、目录与图片授权测试
    geoip.go 可并发切换的本地 MMDB 国家和省州查询
    geoip_update.go GeoIP 自动下载、月度检查、缓存校验及原子更新
    geoip_test.go 下载失败保留缓存、月度回退及官方样本并发查询测试
    http.go 公共 HTTP 入口、运维探针与响应处理
    http_test.go 路由、认证契约、JSON 边界及 SSE 写入错误测试
    install_http.go 原生节点安装资源下载白名单
    lease.go 玩家与节点短期隧道凭据签发及节点幂等校验
    node_http.go 节点登记、认证、同步与领证接口
    node_test.go 节点登记、身份隔离、配置与操作同步测试
    nodes.go 节点登记、配置、操作、撤销与同步
    player_desktop_test.go 桌面房间管理查询与离房房主权限测试
    player_events_http.go 玩家房间事件流与快照恢复
    player_http.go 玩家路由、认证、限速与幂等写入
    player_rooms_http.go 玩家房间查询、成员操作与领证接口
    player_invites.go 邀请元信息、房主按房间重入与条件校验
    player_self.go 当前账号占用、成员墓碑与自身设备查询
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
    tap_windows_test.go 临时用户注册表验证 TAP 两种官方 ID 与专用网卡归属
    tap_other.go 未支持平台的明确错误
  localapi/ 本机服务协议与调用客户端
    client.go 经 Named Pipe/Unix socket 调用本机 RPC
    client_unix_test.go Unix socket 契约外壳、关联标识与错误拒绝测试
    protocol.go 本机请求与节点日志响应类型
  model/ 共享协议类型
    codes.go interaction-1 业务码、HTTP 状态与重试分类
    interaction.go 自身成员、权限、操作收据与独立状态维度
    result.go 统一响应、类型化业务错误与安全详情白名单
    result_test.go 业务码本地化覆盖、权限终态与详情脱敏测试
    update.go 客户端版本、公开下载、更新规则、发布与设备报告结构
    game.go 游戏目录、LAN 策略/心跳、端口区间与管理类型
    lan.go LAN 版本、MAC、完整端口区间校验与 IPv6 地址推导
    model.go 设备、房间、凭据与 LAN 客户端状态类型
    user.go 用户、设备、OIDC 登录与管理请求类型
    node.go 节点配置、登记、操作与同步类型
    version.go 控制面、节点和客户端的独立发布版本
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
    probe_test.go 成员授权、30 秒窗口、全丢包与取消测试
README.md 产品说明、默认配置与操作入口
scripts/ 构建、安装与验证工具
  updates/ 离线更新元数据签署工具
    main_test.go 隔离密钥的完整签名及轮换回归
    main.go 角色密钥、包签名、续签与根轮换
  __pycache__/ Python 字节码缓存（不展开）
  architecture/ 包依赖与文件索引检查
    boundaries_test.go 跨平台生产导入、数据库与数据面归属及客户端运行依赖边界
    files_test.go Git 维护文件与索引的遗漏、重复和失效检查
  build-images.ps1 校验各组件发布包并按独立版本构建镜像，支持显式推送
  build.ps1 按控制面、节点和客户端独立版本构建多架构发布包
  build.sh Linux 可执行文件构建
  desktop/ Windows 与 Linux 桌面构建、交付与原生验收
    build.py 锁定版本的原生 GUI 构建、构建摘要与完整安装包入口
    branding.py 从既有品牌图标几何生成安装向导位图
    Dockerfile Ubuntu 22.04 桌面构建、Windows 交叉编译与 WebView 测试工具
    drivers.py 固定 TAP 驱动与 tapctl 的下载校验、只读提取及对应源码打包
    licenses.py 收集精确解析的桌面依赖声明与许可证
    package.py 校验版本和架构、分离运行文件与临时工具、生成 EXE/deb 与校验和
    windows.nsi 保留玩家账户的双语覆盖安装与独立卸载向导
    locales/ 按语言独立维护的安装向导文案
      zh-CN.nsh 简体中文安装、修复及卸载字典
      en-US.nsh 英文安装、修复及卸载字典
    linux-setup.sh 绑定 Linux 玩家 UID、启动后台与检查本机就绪
    linux-postinst.sh deb 安装或更新后重载并检查已绑定后台
    linux-prerm.sh deb 更新或移除前停止并等待旧后台退出
    linux-postrm.sh deb 移除后重载，保留身份和用户绑定
    test_package.py 版本、二进制架构、内容摘要与 Debian 生命周期测试
    test_live.py 为已打包 Linux 客户端创建和清理隔离联机验收环境
    test-linux.sh 仅容器内的 deb 安装、真实 UID 隔离、替换及卸载验收
    test_webview.py 通过真实 WebView 和本机服务操作玩家完整联机流程
  check-release.py 各组件归档校验和、版本、内容隔离、架构与权限检查
  Install.cmd Windows Go 归档的原生助手双击安装入口
  NodeLaneRoom.cmd 玩家开发命令行启动入口
  openapi/ API 契约生成工具
    main.go 更新协议类型、管理与节点接口及调用方分组
    main_test.go 契约重复生成稳定性与引用完整性检查
  package/ Linux 归档工具
    main.go 生成保留执行权限的 tar.gz
  package-compose.py 部署模板归档与校验和生成
  release/ 原生安装资源打包工具
    main.go 从发布包生成同源下载资源与清单
  test-deploy.py 部署模板与发布镜像冒烟测试
  test-docker.py 隔离 TAP/中继/低 MTU 回归与独立数据库 Go/race 检查
THIRD_PARTY_NOTICES.md 第三方许可声明
```
