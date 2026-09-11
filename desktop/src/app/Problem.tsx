import { WarningCircle } from "@phosphor-icons/react";
import { t } from "../i18n";
import { failure } from "../native/api";
import type { Failure } from "../shared/model";
import { recoveryFor, type Recovery } from "./experience";

const labels = { refresh: "feedback.refreshStatus", network: "experience.retry", diagnostics: "experience.openDiagnostics", account: "experience.openAccount", updates: "experience.openUpdates" } as const;
export function RecoveryButton({ recovery, onRecover, disabled = false }: { recovery: Recovery; onRecover: (action: Recovery) => void; disabled?: boolean }) {
  return <button disabled={disabled} onClick={() => onRecover(recovery)}>{t(labels[recovery])}</button>;
}

export function Problem({ error, onRecover, onDismiss }: { error: Failure; onRecover?: (action: Recovery) => void; onDismiss?: () => void }) {
  const recovery = recoveryFor(error.code);
  return <div className="problem" role="alert">
    <WarningCircle size={22} aria-hidden="true" />
    <div className="problem-content">
      <strong>{t("experience.problemTitle")}</strong>
      <p>{failure(error).error}</p>
      {error.code === "request_state_stale" && <p>{t("experience.staleAction")}</p>}
      <div className="actions compact">
        {recovery && onRecover && <RecoveryButton recovery={recovery} onRecover={onRecover} />}
        {onDismiss && <button className="text-button" onClick={onDismiss}>{t("experience.dismiss")}</button>}
      </div>
      <details className="support-details"><summary>{t("experience.supportDetails")}</summary>
        <span className="mono selectable">{error.code}</span>
        {error.request_id && <span className="mono selectable">{error.request_id}</span>}
      </details>
    </div>
  </div>;
}
