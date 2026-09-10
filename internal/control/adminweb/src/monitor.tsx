import { Card, Badge, Table, number, rate, bytes, date } from "./components";
import {
  latest,
  traffic,
  trafficPoints,
  quality,
  exits,
  measured,
} from "./metrics";
import type { Series, Telemetry } from "./types";

export const modeLabel = (mode: string) =>
  ({ direct: "P2P", relay: "中继", unknown: "未知路径" })[mode] || mode;
const countries = new Intl.DisplayNames(["zh-CN"], { type: "region" });
export function Trend({
  points,
  label,
  unit = "",
  now,
}: {
  points: { at: number; value?: number }[];
  label: string;
  unit?: string;
  now: number;
}) {
  const visible = points.filter(
    (p) => p.at >= now - 60000 && p.at <= now + 1000,
  );
  const max = Math.max(1, ...visible.map((p) => p.value || 0));
  const lines: string[] = [];
  let current = "";
  let previous = 0;
  for (const p of visible) {
    if (p.value == null || (previous && p.at - previous > 15000)) {
      if (current) lines.push(current);
      current = "";
    }
    if (p.value != null) {
      current += `${current ? " L" : "M"}${Math.max(0, ((p.at - now + 60000) / 60000) * 400).toFixed(1)},${(68 - (p.value / max) * 58).toFixed(1)}`;
      previous = p.at;
    }
  }
  if (current) lines.push(current);
  return (
    <div className="trend">
      <div className="trend-label">
        <span>{label}</span>
        <small>
          峰值{" "}
          {number(visible.some((p) => p.value != null) ? max : undefined, unit)}
        </small>
      </div>
      <svg viewBox="0 0 400 80" role="img" aria-label={label + "，最近 60 秒"}>
        <path d="M0,68 H400 M0,39 H400 M0,10 H400" className="grid-line" />
        {lines.map((line, i) => (
          <path key={i} d={line} className="trend-line" />
        ))}
        {!lines.length && (
          <text x="200" y="44" textAnchor="middle">
            等待真实采样
          </text>
        )}
      </svg>
      <div className="trend-axis">
        <span>60 秒前</span>
        <span>现在</span>
      </div>
    </div>
  );
}
export function SeriesMetrics({
  series,
  now,
}: {
  series?: Series;
  now: number;
}) {
  const sample = latest(series, now),
    t = traffic(series, now),
    q = quality(sample?.peers || [], now);
  const tp = trafficPoints(series);
  return (
    <>
      <div className="metrics compact">
        {[
          ["隧道连接", number(sample?.connections)],
          ["上传速率", rate(t.upload)],
          ["下载速率", rate(t.download)],
          ["RTT 范围", `${number(q.min)} / ${number(q.max)} ms`],
          ["平均丢包", number(q.loss, "%")],
          [
            "窗口上传 / 下载",
            `${bytes(t.uploadBytes)} / ${bytes(t.downloadBytes)}`,
          ],
        ].map(([k, v]) => (
          <div className="metric" key={k}>
            <small>{k}</small>
            <strong>{v}</strong>
          </div>
        ))}
      </div>
      <p className="muted">
        {sample
          ? `采样于 ${date(sample.at)}`
          : "没有新鲜采样，可能尚未上报或已离线。"}{" "}
        ·{" "}
        {sample?.traffic?.scope === "nebula_udp"
          ? "Nebula UDP 字节，包含加密、探测与中继转发。"
          : "隧道接口字节，包含游戏与诊断流量。"}{" "}
        流量缺失时显示 —。
      </p>
      <div className="trends">
        <Trend
          now={now}
          label="上传 KiB/s"
          points={tp.map((p) => ({
            at: p.at,
            value: p.upload == null ? undefined : p.upload / 1024,
          }))}
        />
        <Trend
          now={now}
          label="下载 KiB/s"
          points={tp.map((p) => ({
            at: p.at,
            value: p.download == null ? undefined : p.download / 1024,
          }))}
        />
        <Trend
          now={now}
          label="最低 / 最高 RTT 中的最高值"
          unit=" ms"
          points={(series?.samples || []).map((p) => ({
            at: Date.parse(p.at),
            value: quality(p.peers || [], Date.parse(p.at)).max,
          }))}
        />
        <Trend
          now={now}
          label="平均丢包"
          unit="%"
          points={(series?.samples || []).map((p) => ({
            at: Date.parse(p.at),
            value: quality(p.peers || [], Date.parse(p.at)).loss,
          }))}
        />
      </div>
    </>
  );
}
export function Exits({
  device,
  data,
  now,
  names,
}: {
  device: string;
  data?: Telemetry;
  now: number;
  names: Map<string, string>;
}) {
  const observations = exits(device, data, now);
  return (
    <>
      <Table
        heads={["实际远端 IP:端口", "国家 / 地区", "观察来源", "观察时间"]}
        empty={!observations.length}
      >
        {observations.map(({ source, link, at }) => (
          <tr key={source + link.remote}>
            <td className="mono">{link.remote}</td>
            <td>
              {link.country
                ? countries.of(link.country) + " / " + (link.region || "—")
                : "未知"}
            </td>
            <td>{names.get(source) || source.slice(0, 12)}</td>
            <td>{date(at)}</td>
          </tr>
        ))}
      </Table>
      {data?.geoip_provider === "dbip" && (
        <small>
          <a href="https://db-ip.com" target="_blank" rel="noreferrer">
            IP Geolocation by DB-IP
          </a>
        </small>
      )}
      {data?.geoip_provider === "maxmind" && (
        <small>
          GeoIP data by{" "}
          <a href="https://www.maxmind.com" target="_blank" rel="noreferrer">
            MaxMind
          </a>
        </small>
      )}
    </>
  );
}
export function Links({
  series,
  now,
  names,
}: {
  series?: Series;
  now: number;
  names: Map<string, string>;
}) {
  const sample = latest(series, now),
    peers = sample?.peers || [];
  return (
    <>
      <Table
        heads={[
          "对端",
          "路径",
          "对端 IP:端口",
          "实际中继",
          "RTT",
          "丢包",
          "探测时间",
        ]}
        empty={!peers.length}
      >
        {peers.map((p) => (
          <tr key={p.device_id}>
            <td>
              {names.get(p.device_id) || p.device_id.slice(0, 12)}
              <small className="mono">{p.ip}</small>
            </td>
            <td>
              <Badge kind={p.mode === "direct" ? "good" : "warn"}>
                {modeLabel(p.mode)}
              </Badge>
            </td>
            <td className="mono">{p.remote || "未直接观察"}</td>
            <td>{p.mode === "relay" ? p.relay_ips.join(", ") || "—" : "—"}</td>
            <td>{number(measured(p, now) ? p.rtt_ms : undefined, " ms")}</td>
            <td>
              {number(measured(p, now) ? p.loss_percent : undefined, "%")}
            </td>
            <td>{date(p.probe_at)}</td>
          </tr>
        ))}
      </Table>
      {sample && sample.connections > peers.length && (
        <p className="notice">
          共 {sample.connections} 条隧道，本次轮转展示 {peers.length} 条。
        </p>
      )}
    </>
  );
}
export function Monitor({
  series,
  data,
  now,
  names,
}: {
  series?: Series;
  data?: Telemetry;
  now: number;
  names: Map<string, string>;
}) {
  return (
    <>
      <SeriesMetrics series={series} now={now} />
      <Card title="逐条链路">
        <Links series={series} now={now} names={names} />
      </Card>
      {series && (
        <Card title="对端观察到的出口">
          <Exits
            device={series.device_id}
            data={data}
            now={now}
            names={names}
          />
          <p className="muted">
            NAT
            可因目标不同使用多个出口端口；私网地址与未收录地址的归属地显示未知。
          </p>
        </Card>
      )}
    </>
  );
}
