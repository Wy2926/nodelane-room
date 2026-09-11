import { useEffect, useState, type ReactNode } from "react";
import {
  ArrowClockwise,
  ArrowRight,
  CheckCircle,
  CircleDashed,
  Copy,
  Desktop,
  Globe,
  Network,
  Pulse,
  ShieldCheck,
  WarningCircle,
} from "@phosphor-icons/react";
import type { Status, Failure, Diagnostic } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import { formatTime, fresh } from "../../shared/time";
import { PlayerAvatar } from "../../shared/ui/PlayerAvatar";

type Tone = "ok" | "warning" | "neutral";
const controlLabels: Record<string, string> = {
  connected: "已连接",
  unreachable: "无法连接",
  unconfigured: "尚未配置",
  idle: "待连接",
  connecting: "连接中",
};
const controlLabel = (value?: string) => controlLabels[value || ""] || "未知";
const engineLabel = (value?: string) =>
  value === "running" ? "运行中" : value === "stopped" ? "已停止" : "未知";
const measurement = (value?: number) =>
  typeof value === "number" && Number.isFinite(value) && value >= 0;

function CheckState({ tone, children }: { tone: Tone; children: ReactNode }) {
  const Icon =
    tone === "ok"
      ? CheckCircle
      : tone === "warning"
        ? WarningCircle
        : CircleDashed;
  return (
    <span className="check-state" data-tone={tone}>
      <Icon size={18} aria-hidden="true" />
      {children}
    </span>
  );
}

export function Diagnostics({
  status,
  actions,
  usable,
  serviceError,
}: {
  status?: Status;
  actions: Actions;
  usable: boolean;
  serviceError?: Failure;
}) {
  const { perform, copy, busy } = actions;
  const [report, setReport] = useState<{ data: Diagnostic; at: number }>();
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  const available = !!status && !serviceError;
  const running = available && status.engine === "running";
  const expiry = Date.parse(
    status?.ip && status.selected_room ? status.lease_expires_at || "" : "",
  );
  const leaseValid = Number.isFinite(expiry) && expiry > now;
  const current = available && fresh(status.snapshot_at);
  const measured = running && current && leaseValid;
  const peers = (status?.peers || []).filter(
    (peer) => peer.device_id !== status?.device_id,
  );
  const data = report?.data;
  const platform = data?.platform;
  const driver =
    platform?.os === "windows"
      ? platform.tap_interface_present
      : platform?.tun_device_present;
  const connected = available && status.control === "connected";
  const needsAttention =
    !!serviceError ||
    (available &&
      (status.control === "unreachable" ||
        !!status.error ||
        (!!status.selected_room && (!running || !leaseValid))));
  const summary = !available
    ? "等待本机服务"
    : needsAttention
      ? "连接需要检查"
      : running
        ? "游戏网络运行中"
        : "准备好，开始联机";
  const guidance = serviceError
    ? "请确认 NodeLane 网络后台正在运行，然后重试连接。"
    : !status
      ? "正在读取本机连接状态。"
      : status.control === "unreachable"
        ? "控制端暂时不可达，请检查互联网连接与联机服务。"
        : status.selected_room && (!running || !leaseValid)
          ? "房间网络尚未就绪，运行诊断查看系统与授权状态。"
          : running
            ? "在下方查看当前授权和成员之间的实际链路。"
            : "加入房间后，这里会显示虚拟网络与成员链路。";
  const run = () =>
    void perform<Diagnostic>("运行诊断", { action: "doctor" }, (value) =>
      setReport({ data: value, at: Date.now() }),
    );
  const copyReport = () => {
    if (!data || !report) return;
    // Explicit summary fields only; never copy raw errors, identifiers or interface data.
    void copy(
      [
        "NodeLane Room · 脱敏诊断",
        `检查时间：${formatTime(new Date(report.at).toISOString())}`,
        `控制端：${controlLabel(data.control)}`,
        `游戏网络：${engineLabel(data.engine)}`,
        `隧道组件：${driver === undefined ? "未知" : driver ? "已找到" : "未找到"}`,
        `网卡读取：${platform?.interface_error ? "失败" : platform?.interfaces ? "完成" : "未知"}`,
        `后台错误：${data.error ? "存在错误，请在应用内查看" : "未报告"}`,
        "已省略设备标识、IP、网卡、成员和原始错误信息。",
      ].join("\n"),
    );
  };
  return (
    <div className="diagnostics-page">
      <div className="page-intro section-head">
        <div>
          <span className="eyebrow">看清每一段连接</span>
          <h2>网络诊断</h2>
          <p>连接状态、系统环境与实际链路，一目了然。</p>
        </div>
        <button className="primary" disabled={!usable} onClick={run}>
          <ArrowClockwise
            size={21}
            className={busy === "运行诊断" ? "spinning" : undefined}
            aria-hidden="true"
          />
          {busy === "运行诊断" ? "正在检查…" : "运行诊断"}
        </button>
      </div>
      <section
        className="network-overview console-surface"
        aria-label="连接概览"
        data-tone={needsAttention ? "warning" : running ? "ok" : "neutral"}
      >
        <div className="network-summary">
          <span className="network-emblem">
            <Pulse size={35} weight="light" aria-hidden="true" />
          </span>
          <div>
            <h3>{summary}</h3>
            <p>{guidance}</p>
          </div>
        </div>
        <div className="network-path" aria-label="连接状态">
          <div>
            <Desktop size={25} aria-hidden="true" />
            <span>本机服务</span>
            <CheckState
              tone={available ? "ok" : serviceError ? "warning" : "neutral"}
            >
              {available ? "已连接" : serviceError ? "不可用" : "读取中"}
            </CheckState>
          </div>
          <ArrowRight className="path-arrow" size={22} aria-hidden="true" />
          <div>
            <Globe size={25} aria-hidden="true" />
            <span>控制端</span>
            <CheckState
              tone={
                !available
                  ? "neutral"
                  : connected
                    ? "ok"
                    : status.control === "unreachable"
                      ? "warning"
                      : "neutral"
              }
            >
              {available ? controlLabel(status.control) : "未知"}
            </CheckState>
          </div>
          <ArrowRight className="path-arrow" size={22} aria-hidden="true" />
          <div>
            <Network size={25} aria-hidden="true" />
            <span>游戏网络</span>
            <CheckState
              tone={
                running
                  ? "ok"
                  : available && status.selected_room
                    ? "warning"
                    : "neutral"
              }
            >
              {available ? engineLabel(status.engine) : "未知"}
            </CheckState>
          </div>
        </div>
      </section>
      <div className="diagnostic-metrics">
        <div className="console-surface">
          <Network size={23} aria-hidden="true" />
          <span>本机虚拟 IP</span>
          <strong className="mono selectable">
            {available && status.ip ? status.ip : "尚未分配"}
          </strong>
        </div>
        <div className="console-surface">
          <ShieldCheck size={23} aria-hidden="true" />
          <span>当前网络授权</span>
          <strong>
            {!available || !Number.isFinite(expiry)
              ? "暂无授权"
              : leaseValid
                ? `剩余 ${Math.ceil((expiry - now) / 60000)} 分钟`
                : "授权已到期"}
          </strong>
          <small>
            {available && Number.isFinite(expiry)
              ? `截止 ${formatTime(status.lease_expires_at)}`
              : "加入房间后获取"}
          </small>
        </div>
        <div className="console-surface">
          <Pulse size={23} aria-hidden="true" />
          <span>成员链路</span>
          <strong>
            {measured
              ? `${peers.filter((p) => p.mode === "direct" || p.mode === "relay").length} 条已建立`
              : "等待连接"}
          </strong>
          <small>以实际隧道状态为准</small>
        </div>
      </div>
      <div className="diagnostic-columns">
        <section
          className="diagnostic-system console-surface"
          aria-labelledby="system-check-title"
        >
          <div className="diagnostic-section-head">
            <h3 id="system-check-title">系统检查</h3>
            {report && (
              <span className="hint">
                {new Date(report.at).toLocaleTimeString("zh-CN", {
                  hour12: false,
                })}{" "}
                的检查结果
              </span>
            )}
          </div>
          {!report ? (
            <div className="diagnostic-empty">
              <ShieldCheck size={34} weight="light" aria-hidden="true" />
              <h4>给连接做一次体检</h4>
              <p>点击“运行诊断”，检查系统、隧道组件和网络接口。</p>
            </div>
          ) : (
            <>
              <dl className="system-checks">
                <div>
                  <dt>操作系统</dt>
                  <dd>
                    {(
                      {
                        windows: "Windows",
                        linux: "Linux",
                        darwin: "macOS",
                      } as Record<string, string>
                    )[platform?.os || ""] ||
                      platform?.os ||
                      "未知"}
                    <span className="muted"> {platform?.arch || ""}</span>
                  </dd>
                </div>
                <div>
                  <dt>
                    {platform?.os === "windows" ? "LAN 网卡" : "TUN/TAP 设备"}
                  </dt>
                  <dd>
                    <CheckState
                      tone={
                        driver === undefined
                          ? "neutral"
                          : driver
                            ? "ok"
                            : "warning"
                      }
                    >
                      {driver === undefined
                        ? "未获取"
                        : driver
                          ? "已找到"
                          : "未找到"}
                    </CheckState>
                  </dd>
                </div>
                <div>
                  <dt>Nebula 版本</dt>
                  <dd className="mono">{data?.nebula_version || "未知"}</dd>
                </div>
                <div>
                  <dt>网络接口</dt>
                  <dd>
                    <CheckState
                      tone={
                        platform?.interface_error
                          ? "warning"
                          : platform?.interfaces
                            ? "ok"
                            : "neutral"
                      }
                    >
                      {platform?.interface_error
                        ? "读取失败"
                        : platform?.interfaces
                          ? `${platform.interfaces.filter((i) => i.up).length} / ${platform.interfaces.length} 个已启用`
                          : "未获取"}
                    </CheckState>
                  </dd>
                </div>
              </dl>
              {driver === false && (
                <p className="diagnostic-warning">
                  未找到隧道组件，请检查完整客户端是否安装成功。
                </p>
              )}
              {data?.error && (
                <p className="diagnostic-warning">后台报告：{data.error}</p>
              )}
              {!!platform?.interfaces?.length && (
                <details className="interface-details">
                  <summary>查看网络接口</summary>
                  {platform.interfaces.map((item, index) => (
                    <div
                      className="interface-row"
                      key={`${item.name}-${index}`}
                    >
                      <div>
                        <strong>{item.name}</strong>
                        <CheckState tone={item.up ? "ok" : "neutral"}>
                          {item.up ? "已启用" : "已停用"}
                        </CheckState>
                      </div>
                      <p className="mono selectable">
                        {item.addresses?.join(" · ") || "暂无地址"}
                      </p>
                      <small>MTU {item.mtu}</small>
                    </div>
                  ))}
                </details>
              )}
              <div className="diagnostic-copy">
                <button disabled={!!busy} onClick={copyReport}>
                  <Copy size={18} aria-hidden="true" />
                  复制脱敏诊断
                </button>
                <p className="hint">省略设备标识、IP、网卡和成员信息。</p>
              </div>
            </>
          )}
        </section>
        <section
          className="diagnostic-peers console-surface"
          aria-labelledby="peer-check-title"
        >
          <div className="diagnostic-section-head">
            <h3 id="peer-check-title">成员链路</h3>
            <span className="hint">延迟 / 丢包</span>
          </div>
          {!measured && peers.length > 0 && (
            <p className="diagnostic-warning">
              当前状态不可用于判断链路，等待连接和授权同步。
            </p>
          )}
          {!peers.length ? (
            <div className="diagnostic-empty">
              <Network size={34} weight="light" aria-hidden="true" />
              <h4>
                {status?.selected_room ? "等待伙伴加入" : "还没有成员链路"}
              </h4>
              <p>与朋友加入同一房间后，查看直连、中继及实测延迟。</p>
            </div>
          ) : (
            peers.map((peer) => {
              const linked =
                measured && (peer.mode === "direct" || peer.mode === "relay");
              const loss =
                linked &&
                measurement(peer.loss_percent) &&
                peer.loss_percent! <= 100
                  ? peer.loss_percent
                  : undefined;
              const rtt =
                linked && measurement(peer.rtt_ms) ? peer.rtt_ms : undefined;
              return (
                <article className="diagnostic-peer" key={peer.device_id}>
                  <div className="peer-identity">
                    <PlayerAvatar
                      name={peer.name}
                      identity={peer.device_id}
                      size="small"
                    />
                    <strong>{peer.name}</strong>
                    <CheckState tone={linked ? "ok" : "neutral"}>
                      {!measured
                        ? "链路未知"
                        : peer.mode === "direct"
                          ? "直连"
                          : peer.mode === "relay"
                            ? "中继"
                            : "尚未建链"}
                    </CheckState>
                  </div>
                  <div className="peer-measurements">
                    <div>
                      <small>往返延迟</small>
                      <strong>
                        {rtt === undefined ? "未测量" : `${rtt.toFixed(1)} ms`}
                      </strong>
                    </div>
                    <div>
                      <small>丢包率</small>
                      <strong>
                        {loss === undefined ? "未测量" : `${loss.toFixed(0)}%`}
                      </strong>
                    </div>
                    <button
                      className="icon-button"
                      aria-label={`测量 ${peer.name} 的延迟`}
                      title="测量延迟"
                      disabled={!usable || !measured}
                      onClick={() =>
                        void perform("测量延迟", {
                          action: "ping",
                          target: peer.device_id,
                        })
                      }
                    >
                      <Pulse size={22} />
                    </button>
                  </div>
                  {loss !== undefined && (
                    <meter
                      min={0}
                      max={100}
                      value={loss}
                      aria-label={`${peer.name} 的丢包率`}
                    />
                  )}
                </article>
              );
            })
          )}
          <p className="hint peer-note">
            连接类型来自实际隧道；无测量结果时显示“未测量”。
          </p>
        </section>
      </div>
    </div>
  );
}
