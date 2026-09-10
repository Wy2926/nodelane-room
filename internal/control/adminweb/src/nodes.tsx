import { useState, type FormEvent } from "react";
import {
  Action,
  Badge,
  Card,
  Details,
  Field,
  Modal,
  Table,
  date,
  number,
  rate,
} from "./components";
import { Monitor } from "./monitor";
import { latest, quality, traffic } from "./metrics";
import type { API, InfraNode, NodeConfig, Telemetry } from "./types";

export const states: Record<string, string> = {
  pending: "等待登记",
  active: "启用",
  draining: "停止分配",
  disabled: "已停用",
  revoked: "已撤销",
  running: "运行中",
  stopped: "未启动",
  succeeded: "成功",
  failed: "失败",
  expired: "已过期",
  superseded: "已被替代",
};
export function NodeTable({
  nodes,
  telemetry,
  now,
  select,
}: {
  nodes: InfraNode[];
  telemetry?: Telemetry;
  now: number;
  select: (n: InfraNode) => void;
}) {
  return (
    <Table
      heads={[
        "节点 / 公网入口",
        "运行状态",
        "隧道连接",
        "上传 / 下载",
        "RTT 最低 / 最高",
        "平均丢包",
        "操作",
      ]}
      empty={!nodes.length}
    >
      {nodes.map((n) => {
        const s = telemetry?.series.find((s) => s.node_id === n.id),
          sample = latest(s, now),
          t = traffic(s, now),
          q = quality(sample?.peers || [], now),
          connected = Date.parse(n.last_seen) > now - 45000;
        return (
          <tr key={n.id}>
            <td>
              <strong>{n.name}</strong>
              <small>
                {n.region} · {n.address}
              </small>
              <small>
                {n.lighthouse ? "Lighthouse " : ""}
                {n.relay ? "Relay" : ""}
              </small>
            </td>
            <td>
              <Badge
                kind={
                  connected && n.report.engine === "running" ? "good" : "warn"
                }
              >
                {states[n.state] || n.state} ·{" "}
                {connected
                  ? states[n.report.engine || ""] || "等待启动"
                  : "控制离线"}
              </Badge>
              {n.report.error && (
                <small className="error">{n.report.error}</small>
              )}
              <small>
                配置 {n.report.applied_revision || 0} / {n.revision}
              </small>
            </td>
            <td>{number(sample?.connections)}</td>
            <td className="mono">
              ↑ {rate(t.upload)}
              <small>↓ {rate(t.download)}</small>
            </td>
            <td>
              {number(q.min)} / {number(q.max)} ms
            </td>
            <td>{number(q.loss, "%")}</td>
            <td>
              <button onClick={() => select(n)}>详情</button>
            </td>
          </tr>
        );
      })}
    </Table>
  );
}
export function NodeEditor({
  node: initialNode,
  api,
  saved,
}: {
  node?: InfraNode;
  api: API;
  saved: (n: InfraNode) => Promise<void>;
}) {
  // Keep the configuration and its optimistic revision together while editing.
  const [node] = useState(initialNode);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = e.currentTarget;
    setBusy(true);
    setError("");
    const data = new FormData(f);
    let address = String(data.get("address")).trim();
    if (
      !address.includes(":") ||
      (address.startsWith("[") && address.endsWith("]"))
    )
      address += ":4242";
    const config: NodeConfig = {
      name: String(data.get("name")),
      region: String(data.get("region")),
      address,
      notes: String(data.get("notes")),
      lighthouse: data.has("lighthouse"),
      relay: data.has("relay"),
    };
    try {
      const n = await api<InfraNode>(
        node ? "/nodes/" + node.id : "/nodes",
        node ? { config, revision: node.revision } : config,
        node ? "PUT" : "POST",
      );
      await saved(n);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <form onSubmit={submit}>
      <Field
        label="节点名称"
        name="name"
        defaultValue={node?.name}
        required
        maxLength={100}
      />
      <Field
        label="区域"
        name="region"
        defaultValue={node?.region || "default"}
        required
      />
      <Field
        label="公网域名或 IP:端口（默认 UDP 4242）"
        name="address"
        defaultValue={node?.address}
        placeholder="node.example.com:4242"
        required
      />
      <label>
        备注
        <textarea name="notes" defaultValue={node?.notes} />
      </label>
      <div className="checks">
        <Field
          label="地址发现（Lighthouse）"
          name="lighthouse"
          type="checkbox"
          defaultChecked={node?.lighthouse ?? true}
        />
        <Field
          label="中继（Relay）"
          name="relay"
          type="checkbox"
          defaultChecked={node?.relay ?? true}
        />
      </div>
      {node && (
        <p className="notice">
          修改 UDP 端口后，需要在节点本机同步监听端口或 Compose
          映射并应用配置。已有连接可能短暂重连。
        </p>
      )}
      <button disabled={busy} className="primary">
        {busy ? "保存中…" : node ? "保存配置" : "创建节点"}
      </button>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
    </form>
  );
}
export function NodePanel({
  node,
  api,
  refresh,
  close,
  telemetry,
  now,
  names,
  network,
}: {
  node: InfraNode;
  api: API;
  refresh: () => Promise<void>;
  close: () => void;
  telemetry?: Telemetry;
  now: number;
  names: Map<string, string>;
  network: string;
}) {
  const [view, setView] = useState("monitor"),
    [confirm, setConfirm] = useState(""),
    [key, setKey] = useState("");
  const actions: Record<string, [string, string][]> = {
    active: [
      ["drain", "停止分配"],
      ["disable", "停用"],
      ["restart", "重启网络"],
    ],
    draining: [
      ["resume", "恢复分配"],
      ["disable", "停用"],
      ["restart", "重启网络"],
    ],
    disabled: [["resume", "恢复节点"]],
    pending: [["revoke-key", "撤销接入密钥"]],
  };
  const list = [
    ...(actions[node.state] || []),
    ...(node.device_id && node.state !== "revoked"
      ? [["replace", "替换服务器"] as [string, string]]
      : []),
    ...(node.state !== "revoked"
      ? [["revoke", "永久撤销"] as [string, string]]
      : []),
  ];
  const title = list.find((a) => a[0] === confirm)?.[1] || confirm;
  const copy: Record<string, string> = {
    drain: "停止分配新连接，保留身份和续签。",
    disable: "撤销当前数据面证书，保留管理连接。",
    replace: "撤销旧机器身份并隔离旧地址，新机器需要重新登记。",
    revoke: "永久撤销节点身份，已有连接随授权更新而关闭。",
    restart: "重启 Nebula，已有连接将短暂中断。",
  };
  const series = telemetry?.series.find((s) => s.node_id === node.id);
  return (
    <Modal title={node.name} close={close}>
      <div className="tabs">
        {[
          ["monitor", "实时监控"],
          ["config", "配置"],
          ["manage", "管理操作"],
        ].map(([v, l]) => (
          <button
            key={v}
            className={view === v ? "selected" : ""}
            onClick={() => {
              setView(v);
              setKey("");
              setConfirm("");
            }}
          >
            {l}
          </button>
        ))}
      </div>
      {view === "monitor" ? (
        <Monitor series={series} data={telemetry} now={now} names={names} />
      ) : view === "config" ? (
        <NodeEditor
          node={node}
          api={api}
          saved={async () => {
            await refresh();
            setView("monitor");
          }}
        />
      ) : (
        <>
          <Details
            values={{
              节点编号: node.id,
              机器身份: node.device_id || "等待登记",
              身份代次: node.generation,
              "虚拟 IP": node.ip || "—",
              公网入口: node.address,
              最近心跳: date(node.last_seen),
              版本: node.report.version,
              最近续签: date(node.report.last_renewal),
              证书到期: date(node.report.lease_expires_at),
            }}
          />
          <div className="actions">
            {node.state === "pending" && (
              <Action
                className="primary"
                run={async () => {
                  const x = await api<{ key: string }>(
                    "/nodes/" + node.id + "/key",
                    {},
                  );
                  setKey(x.key);
                }}
              >
                生成临时接入密钥
              </Action>
            )}
            {list.map(([a, l]) => (
              <button
                key={a}
                className={a === "revoke" ? "danger" : ""}
                onClick={() => {
                  setConfirm(a);
                  setKey("");
                }}
              >
                {l}
              </button>
            ))}
            <a
              className="button"
              href={"/v2/admin/nodes/" + node.id + "/compose"}
              download="compose.node.yaml"
            >
              下载 Compose YAML
            </a>
          </div>
          {confirm && (
            <Card title={"确认" + title}>
              <p>{copy[confirm] || "将更新配置并等待节点确认。"}</p>
              <div className="actions">
                <Action
                  className="danger"
                  run={async () => {
                    await api("/nodes/" + node.id + "/actions", {
                      action: confirm,
                    });
                    await refresh();
                    setConfirm("");
                  }}
                >
                  确认{title}
                </Action>
                <button onClick={() => setConfirm("")}>取消</button>
              </div>
            </Card>
          )}
          {key && (
            <Card title="临时接入密钥">
              <p className="notice">
                仅本次显示，有效 30 分钟，只能登记一台机器。关闭详情后清除。
              </p>
              <pre className="secret">{key}</pre>
              <Action run={() => navigator.clipboard.writeText(key)}>
                复制密钥
              </Action>
              <h3>宿主机原生安装</h3>
              <pre>{`curl -fsSL ${location.origin}/install/node.sh | bash -s -- --server ${location.origin} --network ${network}`}</pre>
              <p>
                在 Debian/Ubuntu root 终端执行并粘贴密钥。Compose / 1Panel
                部署请下载 YAML，启动后在容器终端执行：
              </p>
              <pre>nlroom-node enroll</pre>
            </Card>
          )}
        </>
      )}
    </Modal>
  );
}
