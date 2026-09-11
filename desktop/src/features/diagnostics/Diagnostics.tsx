import { t, getLanguage, type MessageKey } from "../../i18n";
import { useEffect, useState, type ReactNode } from "react";
import {
  ArrowClockwise,
  CheckCircle,
  CircleDashed,
  Network,
  ShieldCheck,
  WarningCircle,
} from "@phosphor-icons/react";
import type { Status, Failure, Diagnostic } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import { connectionView } from "../../app/experience";

type Tone = "ok" | "warning" | "neutral";
const controlLabels: Record<string, MessageKey> = {
  online: "diagnostics.connected",
  offline: "diagnostics.cannotConnect",
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
  const { perform, busy } = actions;
  const [report, setReport] = useState<{ data: Diagnostic; at: number }>();
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  const available = !!status && !serviceError;
  const expiry = Date.parse(
    status?.ip && status.selected_room ? status.lease_expires_at || "" : "",
  );
  const data = report?.data;
  const platform = data?.platform;
  const driver =
    platform?.os === "windows"
      ? platform.tap_interface_present
      : platform?.tun_device_present;
  const connected = available && status.control === "online";
  const connection = connectionView(status, serviceError, false, now);
  const authorizationExpiry = Math.min(
    expiry,
    Date.parse(status?.membership.valid_until || ""),
  );
  const authorized =
    available &&
    status.membership.state === "active" &&
    authorizationExpiry > now;
  useEffect(() => setReport(undefined), [status?.service_instance_id]);
  const run = () =>
    void perform<Diagnostic>(
      t("diagnostics.runDiagnostics"),
      { action: "doctor" },
      (value) => setReport({ data: value, at: Date.now() }),
    );
  return (
    <div className="diagnostics">
      <div className="page-intro section-head">
        <div>
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
      <div
        className="connection-state diagnostic-summary"
        data-tone={connection.tone}
        role="status"
      >
        <h3>{t(connection.title)}</h3>
        <p>{t(connection.help)}</p>
      </div>
      <div className="diagnostic-grid">
        <section
          className="diagnostic-panel"
          aria-labelledby="connection-check-title"
        >
          <h3 id="connection-check-title">
            <Network size={23} aria-hidden="true" />
            {t("diagnostics.liveConnection")}
          </h3>
          <dl className="system-checks">
            <div>
              <dt>{t("diagnostics.localService")}</dt>
              <dd>
                <CheckState tone={available ? "ok" : "warning"}>
                  {available
                    ? t("diagnostics.connected")
                    : t("diagnostics.unavailable")}
                </CheckState>
              </dd>
            </div>
            <div>
              <dt>{t("diagnostics.controlServiceLabel")}</dt>
              <dd>
                <CheckState tone={connected ? "ok" : "neutral"}>
                  {available
                    ? controlLabel(status.control)
                    : t("diagnostics.unknown")}
                </CheckState>
              </dd>
            </div>
            <div>
              <dt>{t("diagnostics.gameNetworkLabel")}</dt>
              <dd>
                {available
                  ? engineLabel(status.engine)
                  : t("diagnostics.unknown")}
              </dd>
            </div>
            <div>
              <dt>{t("diagnostics.localVirtualIp")}</dt>
              <dd className="mono selectable">
                {authorized && status.ip
                  ? status.ip
                  : t("diagnostics.notAssigned")}
              </dd>
            </div>
            <div>
              <dt>{t("diagnostics.currentNetworkAuthorization")}</dt>
              <dd>
                {!available || !Number.isFinite(expiry)
                  ? t("diagnostics.noAuthorization")
                  : authorized
                    ? t("diagnostics.minRemaining", {
                        0: Math.ceil((authorizationExpiry - now) / 60000),
                      })
                    : t("diagnostics.authorizationExpired")}
              </dd>
            </div>
          </dl>
        </section>
        <section
          className="diagnostic-panel"
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
              </dl>
              {driver === false && (
                <p className="diagnostic-warning">
                  {t("diagnostics.tunnelMissingHelp")}
                </p>
              )}
              {data?.error && (
                <p className="diagnostic-warning">
                  {t("diagnostics.reportProblem")}
                </p>
              )}
              {(!available || now - report.at > 30000) && (
                <p className="hint">{t("diagnostics.reportExpired")}</p>
              )}
            </>
          )}
        </section>
      </div>
    </div>
  );
}
