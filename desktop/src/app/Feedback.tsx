import { t } from "../i18n";
import { failure } from "../native/api";
import type { useService } from "../native/use-service";
import type { Actions } from "./use-actions";
import { formatTime } from "../shared/time";
import { connectionView, type Recovery } from "./experience";
import { Problem, RecoveryButton } from "./Problem";
import { OperationFeedback } from "./OperationFeedback";

export function Feedback({ service, actions, refreshAll, onRecover }: {
  service: ReturnType<typeof useService>; actions: Actions; refreshAll: () => void; onRecover: (action: Recovery) => void;
}) {
  const { status, error, updatedAt, stale, refreshing } = service;
  const view = connectionView(status, error, stale);
  const interrupted = !!error || stale;
  return <>
    {interrupted && <section className="service-notice" data-tone={view.tone} role="alert">
      <div><strong>{t(view.title)}</strong><p>{t(view.help)}</p>
        {error && <p>{failure(error).error}</p>}
        {updatedAt > 0 && <small>{t("experience.lastChecked", { time: formatTime(new Date(updatedAt).toISOString()) })}</small>}
      </div>
      <div className="actions"><button disabled={refreshing} onClick={refreshAll}>{t(refreshing ? "experience.checking" : "feedback.checkAgain")}</button><RecoveryButton recovery="diagnostics" onRecover={onRecover} /></div>
    </section>}
    {!interrupted && ["account", "removed", "update", "reconnecting"].includes(view.kind) && <section className="service-notice" data-tone={view.tone} role="status">
      <div><strong>{t(view.title)}</strong><p>{t(view.help)}</p></div>
      {view.recovery && <RecoveryButton recovery={view.recovery} onRecover={onRecover} />}
    </section>}
    {!actions.dialog && <OperationFeedback actions={actions} />}
    {!actions.dialog && actions.error && !actions.pending && !actions.takeover && <Problem error={actions.error} onRecover={onRecover} onDismiss={() => actions.setError(undefined)} />}
    {actions.notice && <div className="toast" role="status">{t("useActions.copied")}</div>}
  </>;
}
