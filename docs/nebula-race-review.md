# Nebula 竞争与补丁边界

数据面基于官方 v1.11.1，唯一授权补丁为 [d929786cba7f](https://github.com/Wy2926/nebula/commit/d929786cba7f9e69014f88816a326bb0f6e87312)。`go.mod` 固定替换版本；不跟随分支，不修改模块缓存。下列结论来自 2026-09-08 复核，当前工程检查见 [验证记录](validation.md)。

## 已处理

| 竞争 | 原因与处理 |
|---|---|
| 防火墙热更新 | `reloadFirewall` 写入的 firewall 指针被收包协程并发读取。端口规则改变时 Stop/Wait 后重启 Nebula；会短暂重连。普通证书续期仍使用重载。 |
| 握手缓存日志 | `continueHandshake` 在 `Complete` 前求值 `len(hh.packetStore)`，与 `cachePacket` 追加并发。补丁仅把完成步骤移到日志前，使缓存写入在 manager mutex 同步结束后再读取。 |

`StartHandshake` 的缓存回调使用 manager 锁，握手完成路径持有 host 锁；两者不能相互同步。直接在缓存写入时加 host 锁会引入反向锁序。关闭日志等级也不能阻止日志参数求值。

历史复核使用未修改的官方 CLI、独立容器、真实 Linux TUN 与 UDP，两次复现握手缓存竞争，排除了 NodeLane 封装是触发所必需的条件。补丁回归覆盖两种证书版本、pending 移除、主 hostmap 发布和缓存包按序且仅发送一次；聚焦 race 重复运行及补丁 CLI 对照通过。临时原始日志未随源码保存，这些历史结果不能代替后续改动的验收。

## 未处理的上游限制

- `disabledTun.Close` 清空 channel 与 `Read` 并发；已在官方 v1.11.1 复现。NodeLane 原生 TUN 和集成测试 UserDevice 不使用该实现。
- `RemoteList.unlockedSort` 改写 relay slice，与 `relayManager.StartRelays` 读取并发；已在官方 v1.11.1 聚焦 e2e 测试复现，当前补丁未修复。
- 补丁版完整 e2e race 中 `TestRehandshakingRelays` 曾十分钟超时，单独重跑通过；整套运行超时原因未确认。

不能从 NodeLane 全量 race 通过推断 Nebula 全仓库无竞争。CI 仍保留阻塞的全量 race 与真实集成测试，生产验收需继续评估上述限制。
