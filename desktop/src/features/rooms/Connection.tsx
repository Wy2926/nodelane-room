import type { Status } from "../../shared/model";
import type { RoomView } from "./use-room";
import { PortList } from "../../shared/ui/PortList";
import { GameController, PlugsConnected } from "@phosphor-icons/react";

export function Connection({
  view,
  status,
}: {
  view: RoomView;
  status: Status;
}) {
  const { room, game, isCurrent } = view;
  if (!room) return null;
  return (
    <section className="connection" aria-labelledby="connection-title">
      <div className="system-heading">
        <PlugsConnected size={22} weight="light" aria-hidden="true" />
        <h2 id="connection-title">游戏连接</h2>
        <span className="eyebrow">CONNECT</span>
      </div>
      <div className="connection-card console-surface">
        <div className="connection-intro">
          <GameController size={34} weight="light" aria-hidden="true" />
          <div>
            <span className="eyebrow">READY FOR YOUR NEXT GAME</span>
            <h3>游戏内局域网连接</h3>
          </div>
        </div>
        <p className="muted">
          在游戏中选择 NodeLane 网卡并打开局域网列表；也可输入成员的虚拟 IP
          直接加入。游戏监听端口须与配置一致。
        </p>
        {isCurrent && (
          <p className="hint">
            {status.lan?.ready
              ? `LAN 网卡已就绪 · ${status.lan.interface} · MTU ${status.lan.mtu}`
              : "LAN 网卡尚未就绪，请查看网络状态或诊断中的具体错误。"}
          </p>
        )}
        <div className="connection-ports">
          <h3>服务端端口配置</h3>
          <PortList ports={game?.ports || []} />
          <p className="hint">配置由管理员维护，客户端只读。</p>
        </div>
        <details className="connection-help">
          <summary>连接说明</summary>
          <p className="hint">
            端口配置表示网络授权，不代表游戏已启动。广播
            {game?.network.broadcast ? "已开启" : "已关闭"}，组播
            {game?.network.multicast ? "已开启" : "已关闭"}
            。发现和加入还取决于游戏选择的网卡及其实际协议。
          </p>
        </details>
      </div>
    </section>
  );
}
