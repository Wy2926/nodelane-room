import { t, getLanguage, type MessageKey } from "../../i18n";
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
const controlLabels: Record<string, MessageKey> = {
  connected: "diagnostics.connected",
  unreachable: "diagnostics.cannotConnect",
  unconfigured: "settings.notConfigured",
  idle: "diagnostics.idle",
  connecting: "diagnostics.connecting",
};
const controlLabel = (value?: string) =>
  t(controlLabels[value || ""] || "diagnostics.unknown");
const engineLabel = (value?: string) =>
  value === "running"
    ? t("diagnostics.running")
    : value === "stopped"
      ? t("diagnostics.stopped")
      : t("diagnostics.unknown");
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
  const connected = available && status.control === "online";
  const needsAttention =
    !!serviceError ||
    (available &&
      (status.control === "offline" ||
        !!status.error ||
        (!!status.selected_room && (!running || !leaseValid))));
  const summary = !available
    ? t("diagnostics.waitingForTheLocalService")
    : needsAttention
      ? t("diagnostics.connectionNeedsChecking")
      : running
        ? t("diagnostics.gameNetworkIsRunning")
        : t("diagnostics.readyToPlay");
  const guidance = serviceError
    ? t("diagnostics.serviceHelp")
    : !status
      ? t("diagnostics.readingLocalConnectionStatus")
      : status.control === "offline"
        ? t("diagnostics.controlHelp")
        : status.selected_room && (!running || !leaseValid)
          ? t("diagnostics.roomNotReadyHelp")
          : running
            ? t("diagnostics.runningHelp")
            : t("diagnostics.joinRoomHelp");
  const run = () =>
    void perform<Diagnostic>(
      t("diagnostics.runDiagnostics"),
      { action: "doctor" },
      (value) => setReport({ data: value, at: Date.now() }),
    );
  const copyReport = () => {
    if (!data || !report) return;
    // Explicit summary fields only; never copy raw errors, identifiers or interface data.
    void copy(
      [
        t("diagnostics.reportTitle"),
        t("diagnostics.checkedAt", {
          time: formatTime(new Date(report.at).toISOString()),
        }),
        t("diagnostics.controlService", { 0: controlLabel(data.control) }),
        t("diagnostics.gameNetwork", { 0: engineLabel(data.engine) }),
        t("diagnostics.tunnelComponent", {
          0:
            driver === undefined
              ? t("diagnostics.unknown")
              : driver
                ? t("diagnostics.found")
                : t("diagnostics.notFound"),
        }),
        t("diagnostics.networkInterfaces", {
          0: platform?.interface_error
            ? t("diagnostics.failed")
            : platform?.interfaces
              ? t("diagnostics.complete")
              : t("diagnostics.unknown"),
        }),
        t("diagnostics.serviceErrors", {
          0: data.error
            ? t("diagnostics.anErrorWasReportedViewItInThe")
            : t("diagnostics.noneReported"),
        }),
        t("diagnostics.reportOmissions"),
      ].join("\n"),
    );
  };
  return (
    <div className="diagnostics-page">
      <div className="page-intro section-head">
        <div>
          <span className="eyebrow">
            {t("diagnostics.understandEveryConnection")}
          </span>
          <h2>{t("navigation.diagnostics")}</h2>
          <p>{t("diagnostics.intro")}</p>
        </div>
        <button className="primary" disabled={!usable} onClick={run}>
          <ArrowClockwise
            size={21}
            className={
              busy === t("diagnostics.runDiagnostics") ? "spinning" : undefined
            }
            aria-hidden="true"
          />
          {busy === t("diagnostics.runDiagnostics")
            ? t("diagnostics.checking")
            : t("diagnostics.runDiagnostics")}
        </button>
      </div>
      <section
        className="network-overview console-surface"
        aria-label={t("diagnostics.connectionOverview")}
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
        <div
          className="network-path"
          aria-label={t("diagnostics.connectionStatus")}
        >
          <div>
            <Desktop size={25} aria-hidden="true" />
            <span>{t("diagnostics.localService")}</span>
            <CheckState
              tone={available ? "ok" : serviceError ? "warning" : "neutral"}
            >
              {available
                ? t("diagnostics.connected")
                : serviceError
                  ? t("diagnostics.unavailable")
                  : t("diagnostics.loading")}
            </CheckState>
          </div>
          <ArrowRight className="path-arrow" size={22} aria-hidden="true" />
          <div>
            <Globe size={25} aria-hidden="true" />
            <span>{t("diagnostics.controlServiceLabel")}</span>
            <CheckState
              tone={
                !available
                  ? "neutral"
                  : connected
                    ? "ok"
                    : status.control === "offline"
                      ? "warning"
                      : "neutral"
              }
            >
              {available
                ? controlLabel(status.control)
                : t("diagnostics.unknown")}
            </CheckState>
          </div>
          <ArrowRight className="path-arrow" size={22} aria-hidden="true" />
          <div>
            <Network size={25} aria-hidden="true" />
            <span>{t("diagnostics.gameNetworkLabel")}</span>
            <CheckState
              tone={
                running
                  ? "ok"
                  : available && status.selected_room
                    ? "warning"
                    : "neutral"
              }
            >
              {available
                ? engineLabel(status.engine)
                : t("diagnostics.unknown")}
            </CheckState>
          </div>
        </div>
      </section>
      <div className="diagnostic-metrics">
        <div className="console-surface">
          <Network size={23} aria-hidden="true" />
          <span>{t("diagnostics.localVirtualIp")}</span>
          <strong className="mono selectable">
            {available && status.ip ? status.ip : t("diagnostics.notAssigned")}
          </strong>
        </div>
        <div className="console-surface">
          <ShieldCheck size={23} aria-hidden="true" />
          <span>{t("diagnostics.currentNetworkAuthorization")}</span>
          <strong>
            {!available || !Number.isFinite(expiry)
              ? t("diagnostics.noAuthorization")
              : leaseValid
                ? t("diagnostics.minRemaining", {
                    0: Math.ceil((expiry - now) / 60000),
                  })
                : t("diagnostics.authorizationExpired")}
          </strong>
          <small>
            {available && Number.isFinite(expiry)
              ? t("diagnostics.expires", {
                  0: formatTime(status.lease_expires_at),
                })
              : t("diagnostics.availableAfterJoiningARoom")}
          </small>
        </div>
        <div className="console-surface">
          <Pulse size={23} aria-hidden="true" />
          <span>{t("diagnostics.memberConnections")}</span>
          <strong>
            {measured
              ? t("diagnostics.established", {
                  0: peers.filter(
                    (p) => p.mode === "direct" || p.mode === "relay",
                  ).length,
                })
              : t("diagnostics.waitingForConnection")}
          </strong>
          <small>{t("diagnostics.basedOnActualTunnelState")}</small>
        </div>
      </div>
      <div className="diagnostic-columns">
        <section
          className="diagnostic-system console-surface"
          aria-labelledby="system-check-title"
        >
          <div className="diagnostic-section-head">
            <h3 id="system-check-title">{t("diagnostics.systemChecks")}</h3>
            {report && (
              <span className="hint">
                {t("diagnostics.checkedAt", {
                  time: new Date(report.at).toLocaleTimeString(getLanguage(), {
                    hour12: false,
                  }),
                })}
              </span>
            )}
          </div>
          {!report ? (
            <div className="diagnostic-empty">
              <ShieldCheck size={34} weight="light" aria-hidden="true" />
              <h4>{t("diagnostics.giveYourConnectionACheckup")}</h4>
              <p>{t("diagnostics.systemCheckHelp")}</p>
            </div>
          ) : (
            <>
              <dl className="system-checks">
                <div>
                  <dt>{t("diagnostics.operatingSystem")}</dt>
                  <dd>
                    {(
                      {
                        windows: "Windows",
                        linux: "Linux",
                        darwin: "macOS",
                      } as Record<string, string>
                    )[platform?.os || ""] ||
                      platform?.os ||
                      t("diagnostics.unknown")}
                    <span className="muted"> {platform?.arch || ""}</span>
                  </dd>
                </div>
                <div>
                  <dt>
                    {platform?.os === "windows"
                      ? t("diagnostics.lanAdapter")
                      : t("diagnostics.tunTapDevice")}
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
                        ? t("diagnostics.notAvailable")
                        : driver
                          ? t("diagnostics.found")
                          : t("diagnostics.notFound")}
                    </CheckState>
                  </dd>
                </div>
                <div>
                  <dt>{t("diagnostics.nebulaVersion")}</dt>
                  <dd className="mono">
                    {data?.nebula_version || t("diagnostics.unknown")}
                  </dd>
                </div>
                <div>
                  <dt>{t("diagnostics.networkInterfacesLabel")}</dt>
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
                        ? t("diagnostics.readFailed")
                        : platform?.interfaces
                          ? t("diagnostics.enabled", {
                              0: platform.interfaces.filter((i) => i.up).length,
                              1: platform.interfaces.length,
                            })
                          : t("diagnostics.notAvailable")}
                    </CheckState>
                  </dd>
                </div>
              </dl>
              {driver === false && (
                <p className="diagnostic-warning">
                  {t("diagnostics.tunnelMissingHelp")}
                </p>
              )}
              {data?.error && (
                <p className="diagnostic-warning">
                  {t("diagnostics.serviceReport")}
                  {data.error}
                </p>
              )}
              {!!platform?.interfaces?.length && (
                <details className="interface-details">
                  <summary>{t("diagnostics.viewNetworkInterfaces")}</summary>
                  {platform.interfaces.map((item, index) => (
                    <div
                      className="interface-row"
                      key={`${item.name}-${index}`}
                    >
                      <div>
                        <strong>{item.name}</strong>
                        <CheckState tone={item.up ? "ok" : "neutral"}>
                          {item.up
                            ? t("diagnostics.enabledLabel")
                            : t("diagnostics.disabled")}
                        </CheckState>
                      </div>
                      <p className="mono selectable">
                        {item.addresses?.join(" · ") ||
                          t("diagnostics.noAddresses")}
                      </p>
                      <small>MTU {item.mtu}</small>
                    </div>
                  ))}
                </details>
              )}
              <div className="diagnostic-copy">
                <button disabled={!!busy} onClick={copyReport}>
                  <Copy size={18} aria-hidden="true" />
                  {t("diagnostics.copyRedactedDiagnostics")}
                </button>
                <p className="hint">{t("diagnostics.copyHelp")}</p>
              </div>
            </>
          )}
        </section>
        <section
          className="diagnostic-peers console-surface"
          aria-labelledby="peer-check-title"
        >
          <div className="diagnostic-section-head">
            <h3 id="peer-check-title">{t("diagnostics.memberConnections")}</h3>
            <span className="hint">{t("diagnostics.latencyPacketLoss")}</span>
          </div>
          {!measured && peers.length > 0 && (
            <p className="diagnostic-warning">{t("diagnostics.staleHelp")}</p>
          )}
          {!peers.length ? (
            <div className="diagnostic-empty">
              <Network size={34} weight="light" aria-hidden="true" />
              <h4>
                {status?.selected_room
                  ? t("diagnostics.waitingForFriends")
                  : t("diagnostics.noMemberConnectionsYet")}
              </h4>
              <p>{t("diagnostics.peersHelp")}</p>
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
                        ? t("diagnostics.connectionUnknown")
                        : peer.mode === "direct"
                          ? t("diagnostics.direct")
                          : peer.mode === "relay"
                            ? t("diagnostics.relay")
                            : t("diagnostics.notConnected")}
                    </CheckState>
                  </div>
                  <div className="peer-measurements">
                    <div>
                      <small>{t("diagnostics.roundTripLatency")}</small>
                      <strong>
                        {rtt === undefined
                          ? t("diagnostics.notMeasured")
                          : `${rtt.toFixed(1)} ms`}
                      </strong>
                    </div>
                    <div>
                      <small>{t("diagnostics.packetLoss")}</small>
                      <strong>
                        {loss === undefined
                          ? t("diagnostics.notMeasured")
                          : `${loss.toFixed(0)}%`}
                      </strong>
                    </div>
                    <button
                      className="icon-button"
                      aria-label={t("diagnostics.measureLatencyTo", {
                        0: peer.name,
                      })}
                      title={t("diagnostics.measureLatency")}
                      disabled={!usable || !measured}
                      onClick={() =>
                        void perform(t("diagnostics.measureLatency"), {
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
                      aria-label={t("diagnostics.packetLossFor", {
                        0: peer.name,
                      })}
                    />
                  )}
                </article>
              );
            })
          )}
          <p className="hint peer-note">{t("diagnostics.measurementsHelp")}</p>
        </section>
      </div>
    </div>
  );
}
