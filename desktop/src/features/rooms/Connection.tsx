import type { Status } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import type { RoomView } from "./use-room";
import { PortList } from "../../shared/ui/PortList";
import { GameController, PlugsConnected } from "@phosphor-icons/react";
export function Connection({
  view,
  status,
  actions,
  usable,
}: {
  view: RoomView;
  status: Status;
  actions: Actions;
  usable: boolean;
}) {
  const {
    room,
    game,
    isCurrent,
    roomFresh,
    localPorts,
    desiredPorts,
    endpoints,
  } = view;
  const { perform } = actions;
  if (!room) return null;
  return (
    <section className="connection" aria-labelledby="connection-title">
      <div className="system-heading"><PlugsConnected size={22} weight="light" aria-hidden="true" /><h2 id="connection-title">游戏连接</h2><span className="eyebrow">CONNECT</span></div>
      <div className="connection-card console-surface">
      <div className="connection-intro"><GameController size={34} weight="light" aria-hidden="true" /><div><span className="eyebrow">READY FOR YOUR NEXT GAME</span><h3>使用虚拟 IP 直连</h3></div></div>
      <p className="muted">
        在游戏中输入游戏主机的虚拟 IP
        和端口。游戏的实际监听设置须与端口配置一致。
      </p>
      <div className="connection-ports">
      <h3>{room.game === "custom" ? "本机游戏端口" : "服务端端口配置"}</h3>
      {(room.game !== "custom" || isCurrent) && (
        <PortList
          ports={
            room.game === "custom" ? status.ports || [] : game?.ports || []
          }
        />
      )}
      {room.game === "custom" && !isCurrent && (
        <p className="hint">仅管理房间时，请在成员列表查看各自登记的端口。</p>
      )}
      {room.game !== "custom" && (
        <p className="hint">配置由管理员维护，客户端只读。</p>
      )}
      {room.game === "custom" && isCurrent && (
        <>
          <form
            className="port-form"
            onSubmit={(e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              void perform("登记端口", {
                action: "port",
                body: {
                  protocol: f.get("protocol"),
                  port: Number(f.get("port")),
                },
              });
            }}
          >
            <label>
              协议
              <select name="protocol">
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
              </select>
            </label>
            <label>
              端口
              <input
                name="port"
                type="number"
                min={1}
                max={65535}
                required
                placeholder="25565"
              />
            </label>
            <button disabled={!usable || !roomFresh}>添加</button>
          </form>
          {localPorts.map((p) => {
            const desired = desiredPorts.some(
              (e) => e.protocol === p.protocol && e.port === p.port,
            );
            const registered = endpoints.some(
              (e) =>
                e.device_id === status.device_id &&
                e.protocol === p.protocol &&
                e.port === p.port &&
                Date.parse(e.expires_at) > Date.now(),
            );
            return (
              <div className="port-row" key={`${p.protocol}/${p.port}`}>
                <span>
                  {p.protocol.toUpperCase()} {p.port}
                  <small>
                    {!desired
                      ? "已停止续登，等待撤销"
                      : registered && roomFresh
                        ? "已登记"
                        : "待同步"}
                  </small>
                </span>
                <button
                  className="text-button danger"
                  disabled={!usable}
                  onClick={() =>
                    void perform("删除端口", {
                      action: "remove-port",
                      body: {
                        protocol: p.protocol,
                        port: p.port,
                      },
                    })
                  }
                >
                  {desired ? "删除" : "重试删除"}
                </button>
              </div>
            );
          })}
          <p className="hint">
            最多 32 个端口，4243
            保留用于诊断。删除失败时停止续登，已有授权可能保留至原租期结束。
          </p>
        </>
      )}
      </div>
      <details className="connection-help"><summary>连接说明</summary><p className="hint">已登记端口表示网络授权，不代表游戏已启动。当前不提供游戏内 LAN 列表自动发现。</p></details>
      </div>
    </section>
  );
}
