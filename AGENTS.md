# NodeLane Room

Go 游戏组网产品。先读本文件，再按任务检索 [文件索引](docs/files.md) 和相关实现/测试；文档只读相关章节，不沿链接逐份通读。操作见 [README](README.md)，协议与包边界见 [架构](docs/architecture.md)。

## 不可破坏的边界

- 数据面固定官方 `github.com/slackhq/nebula v1.11.1` 及已授权的 `github.com/Wy2926/nebula` 握手缓存 race 补丁，版本由 `go.mod` 的 `replace` 锁定；禁止浮动分支和其他上游升级。仅薄封装生命周期与查询，不自研 VPN、不引入 TURN、Pion 或传输 Provider 抽象；端口规则变化须 Stop/Wait 后重启。
- 设备身份与隧道密钥分离；CA 私钥仅在控制服务。Windows 私密数据须经 DPAPI、ACL 保护，普通用户仅经绑定安装用户 SID 的 Named Pipe 操作服务。禁止记录密钥、会话或邀请码。
- 默认拒绝通信，仅放行同房已登记游戏端口及必要诊断。证书最多 10 分钟；撤销须关闭既有隧道/代理，失联不延长授权，地址回收避开仍有效的旧证书。
- PostgreSQL 为共享状态源；容量、地址、邀请、撤销、幂等与事件持久化须事务一致，SSE 断线可恢复快照，不依赖进程锁保证多副本正确性。初始化仅接受空库或 V2 库，拒绝其他版本且不清库，不加 V1 兼容。
- Minecraft Java 1.21.1 发现使用授权的 Nebula 单播、本机 TCP 代理及 TTL 0 组播回环；校验来源、限速、去重、清理，不以 Nebula 广播/组播代替全房发现。
- 连接类型取自 Nebula 隧道，RTT/丢包取自实际探测，不以候选 relay、心跳或模拟值冒充链路状态。

## 工作方式

- 作最小完整修改，优先标准库及现有实现，清理失效与重复代码；不预建泛化层、为单一实现增加接口或按行数拆包。
- 新增包或改变依赖同步 `scripts/architecture/boundaries_test.go`，不得为保留无必要的跨层依赖放宽规则。
- 新增、删除、移动文件/目录或改变职责时，同步更新 `docs/files.md`；每项一行短述，产物与缓存不展开。
- 代码、注释、文档和输出保持紧凑、直观；文档按职责单处维护，新增前先合并去重，临时排错过程不写入长期规范。
- 接口变更同步更新 `docs/openapi.yaml`；操作变更同步更新 README/部署文档。
- 提交前执行 `gofmt`、`go vet ./...`、`go test ./...`；并发/控制状态改动还须在 Linux 配置独立 `NODELANE_TEST_DATABASE_URL` 后运行 `go test -race -count=1 ./...`。
- 数据面/权限改动须运行真实 Nebula 集成测试；Windows 服务/驱动及双机 NAT/Minecraft 须真机验收。报告实际检查、失败与未验证项（含未运行的数据库测试）；补丁限制与验收见 `docs/validation.md`。
- 产物放 `dist/`，临时内容放 `.local/`；不提交密钥、令牌、生产库或本机工具。不自动申请云资源、发布、安装系统服务或改宿主防火墙。
