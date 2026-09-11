import { useEffect, useState } from "react";
import { t, type MessageKey } from "../../i18n";
import { failure, rpc } from "../../native/api";
import type { UpdateStatus } from "../../shared/model";
import { formatTime } from "../../shared/time";

const stateLabels: Record<string, MessageKey> = {
  idle: "updates.idle", checking: "updates.checking", available: "updates.available", downloading: "updates.downloading", ready: "updates.ready", installing: "updates.installing", succeeded: "updates.succeeded", failed: "updates.failed", rolled_back: "updates.rolledBack", unconfigured: "updates.unconfigured",
};
const errorLabels: Record<string, MessageKey> = { update_metadata_invalid: "updates.invalidSignature", update_trust_unconfigured: "updates.unconfigured", update_disk_full: "updates.diskFull", update_rollback_unavailable: "updates.rollbackUnavailable", update_clock_invalid: "updates.clockInvalid", update_download_failed: "updates.downloadFailed", update_install_failed: "updates.installFailed", update_check_failed: "updates.checkFailed" };

export function Updates() {
  const [value, setValue] = useState<UpdateStatus>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState(false);
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try { const next = await rpc<UpdateStatus>({ action: "update-status" }); if (!stopped) setValue(next); } catch { /* Manual check provides an actionable local error. */ }
      if (!stopped) timer = setTimeout(poll, 2000);
    }
    void poll();
    return () => { stopped = true; clearTimeout(timer); };
  }, []);
  async function act(action: "update-check" | "update-install") {
    setBusy(true); setError("");
    try { await rpc({ action }); setConfirm(false); const next = await rpc<UpdateStatus>({ action: "update-status" }); setValue(next); }
    catch (e) { setError(failure(e).error); }
    finally { setBusy(false); }
  }
  const percent = value?.release ? Math.min(100, Math.floor(value.downloaded / Math.max(1, value.release.size) * 100)) : 0;
  return <div className="update-panel">
    {value?.required && <p role="alert" className="update-message">{t("updates.required")}</p>}
    <p role="status">{value ? t(stateLabels[value.state] || "updates.idle") : t("updates.unknown")}</p>
    {value?.release && <><h4>{t("updates.target", { version: value.release.version })}</h4><p className="update-notes">{value.release.notes}</p></>}
    {value?.policy?.effective_at && <p>{t("updates.deadline", { time: formatTime(value.policy.effective_at) })}</p>}
    {value?.state === "downloading" && <label>{t("updates.progress", { percent: String(percent) })}<progress max={100} value={percent} /></label>}
    {value?.error_code && <p role="alert">{t(errorLabels[value.error_code] || "updates.failed")}</p>}
    {error && <p role="alert">{error}</p>}
    <div className="update-action"><button disabled={busy || value?.state === "downloading" || value?.state === "installing"} onClick={() => void act("update-check")}>{t("settings.checkForUpdates")}</button>
      {value?.state === "ready" && <button disabled={busy} onClick={() => setConfirm(true)}>{t("updates.install")}</button>}</div>
    {confirm && <div className="update-message"><p>{t("updates.installHelp")}</p><button disabled={busy} onClick={() => void act("update-install")}>{t("updates.confirmInstall")}</button><button disabled={busy} onClick={() => setConfirm(false)}>{t("updates.cancel")}</button></div>}
    <p className="muted">{t("updates.behavior")}</p>
  </div>;
}
