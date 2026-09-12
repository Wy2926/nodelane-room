import { useEffect, useRef, useState } from "react";
import { t, type MessageKey } from "../../i18n";
import { clientVersion, failure, rpc } from "../../native/api";
import type { UpdateStatus } from "../../shared/model";
import { Modal } from "../../shared/ui/Modal";
import { formatTime } from "../../shared/time";

const stateLabels: Record<string, MessageKey> = {
  idle: "updates.idle",
  checking: "updates.checking",
  available: "updates.available",
  downloading: "updates.downloading",
  ready: "updates.ready",
  installing: "updates.installing",
  succeeded: "updates.succeeded",
  failed: "updates.failed",
  rolled_back: "updates.rolledBack",
  unconfigured: "updates.unconfigured",
};
const errorLabels: Record<string, MessageKey> = {
  update_metadata_invalid: "updates.invalidSignature",
  update_trust_unconfigured: "updates.unconfigured",
  update_disk_full: "updates.diskFull",
  update_rollback_unavailable: "updates.rollbackUnavailable",
  update_clock_invalid: "updates.clockInvalid",
  update_download_failed: "updates.downloadFailed",
  update_install_failed: "updates.installFailed",
  update_check_failed: "updates.checkFailed",
};

export function Updates() {
  const [value, setValue] = useState<UpdateStatus>();
  const [error, setError] = useState("");
  const [readError, setReadError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const next = await rpc<UpdateStatus>({ action: "update-status" });
        if (!stopped) {
          setValue(next);
          setReadError("");
        }
      } catch (e) {
        if (!stopped) setReadError(failure(e).error);
      }
      if (!stopped) timer = setTimeout(poll, 2000);
    }
    void poll();
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, []);
  async function act(action: "update-check") {
    setBusy(true);
    setError("");
    try {
      await rpc({ action });
      const next = await rpc<UpdateStatus>({ action: "update-status" });
      setValue(next);
      setReadError("");
    } catch (e) {
      setError(failure(e).error);
    } finally {
      setBusy(false);
    }
  }
  const percent = value?.release
    ? Math.min(
        100,
        Math.floor((value.downloaded / Math.max(1, value.release.size)) * 100),
      )
    : 0;
  const ready = value?.state === "ready";
  const available = value?.state === "available";
  const pending =
    busy ||
    ["checking", "downloading", "installing"].includes(value?.state || "");
  const release =
    value && !["idle", "succeeded", "unconfigured"].includes(value.state)
      ? value.release
      : undefined;
  return (
    <section className="update-panel" aria-labelledby="updates-title">
      <div className="update-overview">
        <div>
          <p className="muted">{t("settings.currentVersion")}</p>
          <h3 id="updates-title">v{clientVersion}</h3>
          <p className="update-status" role="status">
            {value
              ? t(stateLabels[value.state] || "updates.unknown")
              : t(readError ? "updates.unknown" : "updates.checking")}
          </p>
        </div>
        <button
          className={ready || available ? "primary" : undefined}
          disabled={pending || (!!readError && ready)}
          onClick={() =>
            available || ready
              ? window.dispatchEvent(new Event("open-update"))
              : void act("update-check")
          }
        >
          {t(
            ready || available ? "updates.install" : "settings.checkForUpdates",
          )}
        </button>
      </div>
      {value?.required && (
        <p role="alert" className="update-message">
          {t("updates.required")}
        </p>
      )}
      {release && (
        <div className="update-release">
          <h4>{t("updates.target", { version: release.version })}</h4>
          {release.notes?.trim() && (
            <details>
              <summary>{t("updates.releaseNotes")}</summary>
              <p className="update-notes">{release.notes}</p>
            </details>
          )}
        </div>
      )}
      {value?.policy?.effective_at && (
        <p className="hint">
          {t("updates.deadline", {
            time: formatTime(value.policy.effective_at),
          })}
        </p>
      )}
      {value?.state === "downloading" && (
        <label>
          {t("updates.progress", { percent: String(percent) })}
          <progress max={100} value={percent} />
        </label>
      )}
      {value?.error_code && (
        <p role="alert">
          {t(errorLabels[value.error_code] || "updates.failed")}
        </p>
      )}
      {(error || readError) && <p role="alert">{error || readError}</p>}
    </section>
  );
}

const skippedVersionKey = "nlroom.skipped-update";
function readSkippedVersion() {
  try {
    return localStorage.getItem(skippedVersionKey) || "";
  } catch {
    return "";
  }
}

export function UpdatePrompt({ enabled = true }: { enabled?: boolean }) {
  const [value, setValue] = useState<UpdateStatus>();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [readError, setReadError] = useState("");
  const [speed, setSpeed] = useState(0);
  const skipped = useRef(readSkippedVersion());
  const dismissed = useRef(new Set<string>());
  const armed = useRef("");
  const sample = useRef({ id: "", state: "", bytes: 0, time: 0 });
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    if (!enabled) return;
    const show = () => {
      setError("");
      setOpen(true);
    };
    window.addEventListener("open-update", show);
    async function poll() {
      try {
        const next = await rpc<UpdateStatus>({ action: "update-status" });
        if (stopped || !next) return;
        setValue(next);
        setReadError("");
        if (["idle", "succeeded"].includes(next.state)) {
          armed.current = "";
          setOpen(false);
        }
        const now = performance.now();
        const old = sample.current;
        setSpeed(
          next.state === "downloading" &&
            old.state === "downloading" &&
            old.id === next.release?.id &&
            now > old.time
            ? Math.max(
                0,
                ((next.downloaded - old.bytes) * 1000) / (now - old.time),
              )
            : 0,
        );
        sample.current = {
          id: next.release?.id || "",
          state: next.state,
          bytes: next.downloaded,
          time: now,
        };
        if (
          next.release &&
          ["available", "ready"].includes(next.state) &&
          !dismissed.current.has(next.release.version) &&
          (skipped.current !== next.release.version || next.required)
        )
          setOpen(true);
        if (armed.current && next.release?.id !== armed.current) {
          armed.current = "";
          setError(t("updates.changed"));
        }
        if (next.state === "ready" && armed.current) {
          armed.current = "";
          setBusy(true);
          try {
            await rpc({ action: "update-install" });
          } catch (e) {
            if (!stopped) setError(failure(e).error);
          } finally {
            if (!stopped) setBusy(false);
          }
        }
        if (["failed", "unconfigured", "rolled_back"].includes(next.state))
          armed.current = "";
      } catch (e) {
        if (!stopped) {
          setSpeed(0);
          setReadError(failure(e).error);
        }
      } finally {
        if (!stopped) timer = setTimeout(poll, 1000);
      }
    }
    void rpc({ action: "update-check" }).catch(() => undefined);
    void poll();
    return () => {
      stopped = true;
      clearTimeout(timer);
      window.removeEventListener("open-update", show);
    };
  }, [enabled]);
  const release = value?.release;
  const downloading = value?.state === "downloading" || !!armed.current;
  const installing = value?.state === "installing";
  async function cancel(skip = false) {
    armed.current = "";
    setBusy(true);
    try {
      await rpc({ action: "update-cancel" });
      if (release) {
        dismissed.current.add(release.version);
        if (skip) {
          skipped.current = release.version;
          try {
            localStorage.setItem(skippedVersionKey, release.version);
          } catch {
            setError(t("updates.skipSession"));
            return;
          }
        }
      }
      setOpen(false);
    } catch (e) {
      setError(failure(e).error);
    } finally {
      setBusy(false);
    }
  }
  async function start() {
    if (!release) return;
    setBusy(true);
    setError("");
    try {
      if (value?.state === "ready") await rpc({ action: "update-install" });
      else {
        armed.current = release.id;
        await rpc({ action: "update-download", target: release.id });
      }
    } catch (e) {
      armed.current = "";
      setError(failure(e).error);
    } finally {
      setBusy(false);
    }
  }
  if (!open || !release) return null;
  const percent = Math.min(
    100,
    Math.floor((value.downloaded * 100) / Math.max(1, release.size)),
  );
  return (
    <Modal
      title={t("updates.newVersion")}
      onClose={() => void cancel()}
      busy={busy || installing}
    >
      <div className="update-prompt">
        <p className="update-version">v{release.version}</p>
        <p className="muted">
          {t("updates.promptHelp", { version: clientVersion })}
        </p>
        {release.notes?.trim() && (
          <div className="update-release">
            <h4>{t("updates.releaseNotes")}</h4>
            <p className="update-notes">{release.notes}</p>
          </div>
        )}
        {value.required && (
          <p className="update-message" role="alert">
            {t("updates.required")}
          </p>
        )}
        <p className="hint">{t("updates.autoInstallHelp")}</p>
        {downloading && (
          <div className="update-download" role="status">
            <label>
              {t("updates.progress", { percent: String(percent) })}
              <progress max={100} value={percent} />
            </label>
            <p className="muted">
              {t("updates.transfer", {
                downloaded: (value.downloaded / 1048576).toFixed(1),
                total: (release.size / 1048576).toFixed(1),
                speed: (speed / 1048576).toFixed(2),
              })}
            </p>
            {percent === 100 && <p>{t("updates.preparing")}</p>}
          </div>
        )}
        {installing && <p role="status">{t("updates.installing")}</p>}
        {value.error_code && (
          <p role="alert">{failure({ code: value.error_code }).error}</p>
        )}
        {(error || readError) && <p role="alert">{error || readError}</p>}
        <div className="actions update-prompt-actions">
          <button disabled={busy || installing} onClick={() => void cancel()}>
            {t("updates.cancel")}
          </button>
          {!downloading && !installing && (
            <>
              <button
                disabled={busy || value.required}
                onClick={() => void cancel(true)}
              >
                {t("updates.skipVersion")}
              </button>
              <button
                className="primary"
                disabled={busy}
                onClick={() => void start()}
              >
                {t("updates.install")}
              </button>
            </>
          )}
        </div>
      </div>
    </Modal>
  );
}
