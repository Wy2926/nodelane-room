import { useEffect, useState } from "react";
import {
  Action,
  Badge,
  Card,
  Details,
  Modal,
  Table,
  number,
  rate,
  date,
} from "./components";
import { Monitor, modeLabel, Exits, Trend } from "./monitor";
import { latest, quality, roomMetrics, roomHistory, traffic } from "./metrics";
import type { API, Room, RoomSnapshot, Telemetry } from "./types";

export function RoomTable({
  rooms,
  telemetry,
  now,
  nodeDevices,
  select,
}: {
  rooms: Room[];
  telemetry?: Telemetry;
  now: number;
  nodeDevices: Set<string>;
  select: (r: Room) => void;
}) {
  return (
    <Table
      heads={[
        "房间",
        "状态",
        "成员间连接",
        "上传 / 下载",
        "最低 / 最高 RTT",
        "P2P / 中继",
        "操作",
      ]}
      empty={!rooms.length}
    >
      {rooms.map((r) => {
        const m = roomMetrics(r.id, telemetry, nodeDevices, now);
        return (
          <tr key={r.id}>
            <td>
              <strong>{r.name}</strong>
              <small>{r.game_name}</small>
            </td>
            <td>
              <Badge
                kind={!r.closed && Date.parse(r.expires_at) > now ? "good" : ""}
              >
                {r.closed
                  ? "已关闭"
                  : Date.parse(r.expires_at) > now
                    ? "开放中"
                    : "已过期"}
              </Badge>
            </td>
            <td>
              {number(m.connections)}
              <small>{m.reporting} 位成员已上报</small>
            </td>
            <td className="mono">
              ↑ {rate(m.upload)}
              <small>↓ {rate(m.download)}</small>
            </td>
            <td>
              {number(m.min)} / {number(m.max)} ms
            </td>
            <td>
              {m.direct} / {m.relay}
            </td>
            <td>
              <button onClick={() => select(r)}>查看成员</button>
            </td>
          </tr>
        );
      })}
    </Table>
  );
}
export function RoomPanel({
  room,
  api,
  close,
  refresh,
  telemetry,
  now,
  nodeDevices,
  nodeNames,
}: {
  room: Room;
  api: API;
  close: () => void;
  refresh: () => Promise<void>;
  telemetry?: Telemetry;
  now: number;
  nodeDevices: Set<string>;
  nodeNames: Map<string, string>;
}) {
  const [snapshot, setSnapshot] = useState<RoomSnapshot>(),
    [error, setError] = useState(""),
    [selected, setSelected] = useState(""),
    [confirm, setConfirm] = useState<{
      action: string;
      device: string;
      revision: number;
    }>();
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const x = await api<RoomSnapshot>(
          "/rooms/" + room.id,
          undefined,
          "GET",
          controller.signal,
        );
        setSnapshot(x);
        setError("");
      } catch (e) {
        if (!controller.signal.aborted) setError((e as Error).message);
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(poll, 5000);
      }
    }
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [api, room.id]);
  const members = snapshot?.members || [],
    ids = new Set(members.map((m) => m.device_id)),
    names = new Map([
      ...nodeNames,
      ...members.map((m) => [m.device_id, m.name] as [string, string]),
    ]);
  const metrics = roomMetrics(room.id, telemetry, nodeDevices, now, ids);
  const history = roomHistory(room.id, telemetry, nodeDevices, now, ids);
  const series = (id: string) =>
    telemetry?.series.find((s) => s.device_id === id && s.room_id === room.id);
  return (
    <Modal title={room.name} close={close}>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {!snapshot ? (
        <p>正在读取成员…</p>
      ) : (
        <>
          <Details
            values={{
              游戏: room.game,
              成员: members.length,
              成员间连接: number(metrics.connections),
              "P2P / 中继": `${metrics.direct} / ${metrics.relay}`,
              "上传 / 下载": `${rate(metrics.upload)} / ${rate(metrics.download)}`,
              "RTT 最低 / 最高": `${number(metrics.min)} / ${number(metrics.max)} ms`,
              平均链路丢包: number(metrics.loss, "%"),
              到期时间: date(room.expires_at),
            }}
          />
          <p className="notice">
            {metrics.reporting} / {members.length} 位成员有新鲜上报，
            {metrics.trafficReporting}{" "}
            位可计算速率。房间连接按成员对去重；流量是成员接口之和，同一传输会分别计入发送方上传和接收方下载。
          </p>
          <div className="trends">
            <Trend
              now={now}
              label="房间上传 KiB/s"
              points={history.map((p) => ({
                at: p.at,
                value: p.upload == null ? undefined : p.upload / 1024,
              }))}
            />
            <Trend
              now={now}
              label="房间下载 KiB/s"
              points={history.map((p) => ({
                at: p.at,
                value: p.download == null ? undefined : p.download / 1024,
              }))}
            />
            <Trend
              now={now}
              label="房间最低 RTT"
              unit=" ms"
              points={history.map((p) => ({ at: p.at, value: p.min }))}
            />
            <Trend
              now={now}
              label="房间最高 RTT"
              unit=" ms"
              points={history.map((p) => ({ at: p.at, value: p.max }))}
            />
          </div>
          <Table
            heads={[
              "成员",
              "隧道连接",
              "上传 / 下载",
              "RTT 范围",
              "平均丢包",
              "操作",
            ]}
            empty={!members.length}
          >
            {members.map((m) => {
              const s = series(m.device_id),
                sample = latest(s, now),
                t = traffic(s, now),
                q = quality(sample?.peers || [], now);
              return (
                <tr key={m.device_id}>
                  <td>
                    <strong>{m.name}</strong>
                    <small className="mono">{m.ip}</small>
                    <small>
                      {m.user_id === room.owner_user_id ? "房主" : ""}{" "}
                      {date(m.last_seen)}
                    </small>
                  </td>
                  <td>{number(sample?.connections)}</td>
                  <td>
                    ↑ {rate(t.upload)}
                    <small>↓ {rate(t.download)}</small>
                  </td>
                  <td>
                    {number(q.min)} / {number(q.max)} ms
                  </td>
                  <td>{number(q.loss, "%")}</td>
                  <td>
                    <div className="actions">
                      <button
                        onClick={() =>
                          setSelected(
                            selected === m.device_id ? "" : m.device_id,
                          )
                        }
                      >
                        {selected === m.device_id ? "收起" : "链路与出口"}
                      </button>
                      <button
                        className="danger"
                        onClick={() =>
                          setConfirm({
                            action: "kick",
                            device: m.device_id,
                            revision: snapshot.room.revision,
                          })
                        }
                      >
                        踢出
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
          </Table>
          {selected && ids.has(selected) && (
            <Card title={names.get(selected) + " · 最近 60 秒"}>
              <Monitor
                series={series(selected)}
                data={telemetry}
                now={now}
                names={names}
              />
              {!series(selected) && (
                <Exits
                  device={selected}
                  data={telemetry}
                  now={now}
                  names={names}
                />
              )}
            </Card>
          )}
          <Card title="房间实际连接">
            <Table
              heads={["成员对", "最近观察路径", "实际中继 IP", "RTT / 丢包"]}
              empty={!metrics.links.length}
            >
              {metrics.links.map(({ source, peer }) => (
                <tr key={[source, peer.device_id].sort().join("/")}>
                  <td>
                    {names.get(source)} ↔{" "}
                    {names.get(peer.device_id) || peer.device_id.slice(0, 12)}
                  </td>
                  <td>
                    <Badge kind={peer.mode === "direct" ? "good" : "warn"}>
                      {modeLabel(peer.mode)}
                    </Badge>
                  </td>
                  <td>
                    {peer.mode === "relay"
                      ? peer.relay_ips.join(", ") || "—"
                      : "—"}
                  </td>
                  <td>
                    {number(peer.rtt_ms, " ms")} /{" "}
                    {number(peer.loss_percent, "%")}
                  </td>
                </tr>
              ))}
            </Table>
            <p className="muted">
              路径是最近一次定向观察；两端可能在切换瞬间不同。逐方向数据见成员详情。
            </p>
          </Card>
          <Card title="游戏网络授权">
            <p>
              广播{snapshot.game.network.broadcast ? "已开启" : "已关闭"} · 组播
              {snapshot.game.network.multicast ? "已开启" : "已关闭"}
            </p>
            <Table
              heads={["协议", "端口范围", "用途"]}
              empty={!snapshot.game.ports.length}
            >
              {snapshot.game.ports.map((p) => (
                <tr key={`${p.protocol}/${p.port}`}>
                  <td>{p.protocol}</td>
                  <td>
                    {p.port}
                    {p.port_end ? `–${p.port_end}` : ""}
                  </td>
                  <td>{p.description}</td>
                </tr>
              ))}
            </Table>
          </Card>
          {!room.closed && (
            <button
              className="danger"
              onClick={() =>
                setConfirm({
                  action: "close",
                  device: "",
                  revision: snapshot.room.revision,
                })
              }
            >
              关闭房间
            </button>
          )}
          {confirm && (
            <Card
              title={
                confirm.action === "close"
                  ? "确认关闭房间"
                  : "确认踢出 " + names.get(confirm.device)
              }
            >
              <p>此操作会撤销相关授权并关闭已有连接。</p>
              <div className="actions">
                <Action
                  className="danger"
                  run={async () => {
                    await api("/rooms/" + room.id + "/actions", {
                      action: confirm.action,
                      device_id: confirm.device,
                      expected_revision: confirm.revision,
                    });
                    setConfirm(undefined);
                    await refresh();
                    const s = await api<RoomSnapshot>("/rooms/" + room.id);
                    setSnapshot(s);
                  }}
                >
                  确认执行
                </Action>
                <button onClick={() => setConfirm(undefined)}>取消</button>
              </div>
            </Card>
          )}
        </>
      )}
    </Modal>
  );
}
