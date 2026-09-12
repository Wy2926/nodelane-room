import { t } from "../i18n";
import type { Actions } from "./use-actions";

export function OperationFeedback({ actions }: { actions: Actions }) {
  if (actions.takeover) return <div className="operation-card" role="alert">
    <strong>{t("interaction.takeoverHelp")}</strong>
    <p>{t("account.takeoverHelp")}</p>
    <small>{actions.takeover.device}</small>
    <div className="actions"><button className="primary" disabled={!!actions.busy} onClick={() => void actions.takeOverAndContinue()}>{t("interaction.takeover")}</button><button disabled={!!actions.busy} onClick={actions.cancelTakeover}>{t("account.cancel")}</button></div>
  </div>;
  if (actions.pending) return <div className="operation-card" role="status">
    <strong>{t(actions.unresolved ? "experience.unresolvedTitle" : "experience.pendingTitle")}</strong>
    <p>{t(actions.unresolved ? "experience.unresolvedHelp" : "experience.pendingHelp")}</p>
    <div className="actions">
      <button disabled={actions.checking || !!actions.busy} onClick={() => void actions.checkOperation()}>{t(actions.checking ? "experience.checking" : "experience.pendingCheck")}</button>
      <button onClick={() => void actions.perform(t("interaction.stopNetwork"), { action: "network-stop" })}>{t("interaction.stopNetwork")}</button>
      {actions.unresolved && <button onClick={() => void actions.reviewPending()}>{t(actions.reviewed ? "experience.confirmReviewed" : "experience.reviewState")}</button>}
    </div>
  </div>;
  if (actions.busy) return <div className="operation-progress" role="status"><span className="activity-dot" aria-hidden="true" /><div><strong>{actions.busy}…</strong><p>{t("experience.workingHelp")}</p></div></div>;
  return null;
}
