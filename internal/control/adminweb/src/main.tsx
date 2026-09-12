import { StrictMode, useCallback, useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { makeAPI, watchAdmin, businessMessage } from "./api";
import { PendingOperations } from "./operations";
import { Auth } from "./auth";
import { Action, Badge, Card, Details, Modal, Table, date } from "./components";
import { NodeEditor, NodePanel, NodeTable, states } from "./nodes";
import { RoomPanel, RoomTable } from "./rooms";
import { Users } from "./users";
import { OIDCSettings, Password } from "./settings";
import { Navigation, pages, useAdminPage } from "./navigation";
import { UpdateSources } from "./update-sources";
import { UpdatePolicies } from "./update-policies";
import { UpdateDevices } from "./update-devices";
import { Games } from "./games";
import { Releases } from "./updates";
import type { Room, Session, Snapshot, Telemetry } from "./types";
import "./style.css";

export function App() {
  const tab = useAdminPage();
  const page = pages.find((page) => page.id === tab)!;
  const monitored = ["overview", "nodes", "rooms"].includes(tab);
  const [session, setSession] = useState<Session>(),
    [restored, setRestored] = useState(false),
    [setup, setSetup] = useState(false),
    [message, setMessage] = useState(""),
    [data, setData] = useState<Snapshot>(),
    [telemetry, setTelemetry] = useState<Telemetry>(),
    [received, setReceived] = useState(0),
    [tick, setTick] = useState(Date.now()),
    [connection, setConnection] = useState("正在同步"),
    [monitorError, setMonitorError] = useState(""),
    [nodeID, setNodeID] = useState(""),
    [room, setRoom] = useState<Room>(),
    [addNode, setAddNode] = useState(false);
  useEffect(() => {
    setNodeID("");
    setRoom(undefined);
    setAddNode(false);
    document.title = `${page.label} · NodeLane Room 管理台`;
  }, [tab, page.label]);
  const signedOut = useCallback(() => {
    setSession(undefined);
    setData(undefined);
    setTelemetry(undefined);
    setNodeID("");
    setRoom(undefined);
    setAddNode(false);
  }, []);
  const api = useMemo(
    () => makeAPI(session?.csrf || "", signedOut),
    [session?.csrf, signedOut],
  );
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function restore() {
      try {
        const s = await api<{ initialized: boolean; configured: boolean }>(
          "/setup",
          undefined,
          "GET",
          controller.signal,
        );
        if (s.initialized) {
          try {
            const restoredSession = await api<Session>(
              "/session",
              undefined,
              "GET",
              controller.signal,
            );
            if (!controller.signal.aborted) setSession(restoredSession);
          } catch {
            /* Login is shown for an expired session. */
          }
          if (!controller.signal.aborted) setRestored(true);
        } else if (!s.configured) {
          setSetup(true);
          setRestored(true);
        } else {
          setMessage("正在连接已配置的数据库…");
          timer = setTimeout(restore, 2000);
        }
      } catch (e) {
        if (!controller.signal.aborted) {
          setMessage((e as Error).message);
          setRestored(true);
        }
      }
    }
    void restore();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
    // Restore once; the authenticated stream owns subsequent session checks.
  }, []);
  const refresh = useCallback(async () => {
    const x = await api<Snapshot>("/snapshot");
    setData(x);
  }, [api]);
  useEffect(() => {
    if (!session) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const stream = watchAdmin(
      (value) => {
        setData(value as Snapshot);
        setConnection("控制端已连接");
      },
      () => {
        setConnection("连接中断，正在重试");
        void api("/session", undefined, "GET", controller.signal).catch(
          () => {},
        );
      },
      controller.signal,
    );
    void api<Snapshot>("/snapshot", undefined, "GET", controller.signal)
      .then((x) => {
        if (!controller.signal.aborted) setData(x);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setMessage(e.message);
      });
    async function poll() {
      try {
        const t = await api<Telemetry>(
          "/telemetry",
          undefined,
          "GET",
          controller.signal,
        );
        if (!controller.signal.aborted) {
          setTelemetry(t);
          setReceived(Date.now());
          setMonitorError("");
        }
      } catch (e) {
        if (!controller.signal.aborted) setMonitorError((e as Error).message);
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(poll, 5000);
      }
    }
    void poll();
    return () => {
      controller.abort();
      stream.close();
      clearTimeout(timer);
    };
  }, [session, api, refresh]);
  useEffect(() => {
    const timer = setInterval(() => setTick(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  const now = telemetry
    ? Date.parse(telemetry.server_time) + tick - received
    : tick;
  const names = new Map((data?.nodes || []).map((n) => [n.device_id, n.name]));
  const nodeDevices = new Set((data?.nodes || []).map((n) => n.device_id));
  const selectedNode = data?.nodes.find((n) => n.id === nodeID);
  if (!restored)
    return <div className="loading">{message || "正在连接控制端…"}</div>;
  if (!session)
    return (
      <Auth
        api={api}
        initiallySetup={setup}
        initialMessage={message}
        signedIn={(s) => {
          setMessage("");
          setSetup(false);
          setSession(s);
        }}
      />
    );
  return (
    <div className="app">
      <aside>
        <a className="brand" href="#overview">
          <b>N</b>
          <span>
            NodeLane<small>ROOM CONTROL</small>
          </span>
        </a>
        <Navigation current={tab} />
        <div className="sidebar-foot">
          <a className="sidebar-site" href="/">
            访问官网 ↗
          </a>
          <Badge kind={connection === "控制端已连接" ? "good" : "warn"}>
            {connection}
          </Badge>
          <small>
            {session.username}
            {data?.version ? ` · v${data.version}` : ""}
          </small>
          <Action
            run={async () => {
              try {
                await api("/logout", {});
              } finally {
                signedOut();
              }
            }}
          >
            退出登录
          </Action>
        </div>
      </aside>
      <main>
        <PendingOperations api={api} refresh={refresh} />
        {data?.truncated &&
          ((["overview", "rooms"].includes(tab) && data.truncated.rooms) ||
            (tab === "events" &&
              (data.truncated.operations || data.truncated.events))) && (
            <p role="status">
              部分列表仅显示最近记录；缺项不代表已删除或操作未执行，请按编号查询。
            </p>
          )}
        <header>
          <div>
            <p className="eyebrow">NODELANE / CONTROL CENTER</p>
            <h1>{page.label}</h1>
            <p className="muted">{page.description}</p>
          </div>
          {monitored && (
            <div className="header-status">
              <Badge kind="good">内存窗口 · 60 秒</Badge>
              <small>每 5 秒更新 · {date(telemetry?.server_time)}</small>
            </div>
          )}
        </header>
        {message && (
          <p className="error" role="alert">
            {message}
          </p>
        )}
        {monitored && monitorError && (
          <p className="error" role="alert">
            监控读取失败：{monitorError}。过期样本将停止显示。
          </p>
        )}
        {!data ? (
          <Card>
            <p>正在加载控制状态…</p>
            <Action run={refresh}>重试</Action>
          </Card>
        ) : (
          <>
            {(tab === "overview" || tab === "nodes") && (
              <>
                {tab === "overview" && (
                  <div className="metrics overview">
                    {[
                      [
                        "运行节点",
                        data.nodes.filter(
                          (n) =>
                            Date.parse(n.last_seen) > now - 45000 &&
                            n.report.engine === "running" &&
                            ["active", "draining"].includes(n.state),
                        ).length,
                        "真实引擎与控制状态",
                      ],
                      [
                        "开放房间",
                        data.rooms.filter(
                          (r) => !r.closed && Date.parse(r.expires_at) > now,
                        ).length,
                        "每房默认 4 位成员",
                      ],
                      [
                        "正在上报",
                        telemetry?.series.filter(
                          (s) =>
                            Date.parse(s.samples.at(-1)?.at || "") >
                            now - 15000,
                        ).length ?? 0,
                        "节点与成员，15 秒内新鲜",
                      ],
                      ["观察窗口", "60s", "重启清空 · 不写入数据库"],
                    ].map(([label, value, hint]) => (
                      <Card key={label}>
                        <small>{label}</small>
                        <strong className="big-metric">{value}</strong>
                        <small>{hint}</small>
                      </Card>
                    ))}
                  </div>
                )}
                <Card>
                  <div className="section-heading">
                    <div>
                      <h2>基础设施节点</h2>
                      <p className="muted">连接、带宽与真实探测</p>
                    </div>
                    <button
                      className="primary"
                      onClick={() => setAddNode(true)}
                    >
                      ＋ 添加节点
                    </button>
                  </div>
                  <NodeTable
                    nodes={data.nodes}
                    telemetry={telemetry}
                    now={now}
                    select={(n) => setNodeID(n.id)}
                  />
                </Card>
              </>
            )}
            {(tab === "overview" || tab === "rooms") && (
              <Card title="房间网络">
                <RoomTable
                  rooms={data.rooms}
                  telemetry={telemetry}
                  now={now}
                  nodeDevices={nodeDevices}
                  select={setRoom}
                />
              </Card>
            )}
            {tab === "users" && <Users api={api} />}
            {tab === "oidc" && (
              <OIDCSettings api={api} publicURL={data.public_url} />
            )}
            {tab === "sources" && <UpdateSources api={api} />}
            {tab === "policies" && <UpdatePolicies api={api} />}
            {tab === "devices" && <UpdateDevices api={api} />}
            {tab === "releases" && <Releases api={api} />}
            {tab === "games" && (
              <Games games={data.games} api={api} refresh={refresh} />
            )}
            {tab === "events" && (
              <>
                <Card title="节点操作结果">
                  <Table
                    heads={["动作", "节点", "状态", "创建时间", "错误"]}
                    empty={!data.operations.length}
                  >
                    {data.operations.map((o) => (
                      <tr key={o.id}>
                        <td>{o.action}</td>
                        <td>
                          {data.nodes.find((n) => n.id === o.node_id)?.name ||
                            o.node_id}
                        </td>
                        <td>
                          <Badge kind={o.state === "failed" ? "bad" : ""}>
                            {states[o.state] || o.state}
                          </Badge>
                        </td>
                        <td>{date(o.created_at)}</td>
                        <td>
                          {o.reason || o.error
                            ? businessMessage(o.reason || o.error!)
                            : "—"}
                        </td>
                      </tr>
                    ))}
                  </Table>
                </Card>
                <Card title="审计记录">
                  <Table
                    heads={["时间", "操作者", "事件", "目标", "内容"]}
                    empty={!data.events.length}
                  >
                    {data.events.map((e) => (
                      <tr key={e.id}>
                        <td>{date(e.created_at)}</td>
                        <td>{e.actor}</td>
                        <td>{e.kind}</td>
                        <td>{e.target}</td>
                        <td>
                          <details>
                            <summary>查看</summary>
                            <pre>{JSON.stringify(e.detail, null, 2)}</pre>
                          </details>
                        </td>
                      </tr>
                    ))}
                  </Table>
                </Card>
              </>
            )}
            {tab === "deployment" && (
              <Card title="部署信息">
                <Details
                  values={{
                    版本: data.version,
                    部署身份: data.deployment_id,
                    公网地址: data.public_url,
                    节点镜像仓库: data.registry,
                    游戏地址池: data.network,
                    "CA 到期": date(data.ca_expires_at),
                    "GeoIP 数据库": telemetry?.geoip
                      ? "已加载本地数据库"
                      : "尚未就绪；自动下载成功后显示归属地",
                    数据库: "PostgreSQL 共享控制状态",
                    实时监控: "当前控制实例内存保留 60 秒，重启清空",
                  }}
                />
                <p className="notice">
                  多个控制实例各自持有接收到的监控样本；请通过固定实例查看完整窗口。公网
                  IP 归属地由本地 GeoIP 数据库提供，IP 不发送给第三方服务。
                </p>
              </Card>
            )}
            {tab === "password" && (
              <Card title="修改管理员密码">
                <Password
                  api={api}
                  done={() => {
                    signedOut();
                    setMessage("密码已修改，请重新登录。");
                  }}
                />
              </Card>
            )}
            {Date.parse(data.ca_expires_at) - now < 30 * 86400000 && (
              <p className="notice">Nebula CA 将在 30 天内到期，请安排维护。</p>
            )}
            {monitored && (
              <footer>
                — 表示缺少有效采样。连接类型来自 Nebula
                隧道，延迟与丢包来自实际探测。
                {!telemetry?.geoip
                  ? " IP 归属库尚未就绪，自动下载成功后显示。"
                  : ""}
              </footer>
            )}
          </>
        )}
        {selectedNode && data && (
          <NodePanel
            node={selectedNode}
            api={api}
            refresh={refresh}
            close={() => setNodeID("")}
            telemetry={telemetry}
            now={now}
            names={names}
            network={data.network}
          />
        )}{" "}
        {room && (
          <RoomPanel
            room={data?.rooms.find((r) => r.id === room.id) || room}
            api={api}
            close={() => setRoom(undefined)}
            refresh={refresh}
            telemetry={telemetry}
            now={now}
            nodeDevices={nodeDevices}
            nodeNames={names}
          />
        )}{" "}
        {addNode && (
          <Modal title="添加节点" close={() => setAddNode(false)}>
            <NodeEditor
              api={api}
              saved={async (n) => {
                await refresh();
                setAddNode(false);
                setNodeID(n.id);
              }}
            />
          </Modal>
        )}
      </main>
    </div>
  );
}

const root = document.getElementById("root");
if (root)
  createRoot(root).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
