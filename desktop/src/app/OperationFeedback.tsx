import { t } from "../i18n";
import type { Actions } from "./use-actions";
import { Loading, Spinner } from "../shared/ui/Loading";

export function OperationFeedback({ actions }: { actions: Actions }) {
  if (actions.takeover) return <div className="operation-card" role="alert">
    <strong>{t("interaction.takeoverHelp")}</strong>
    <p>{t("account.takeoverHelp")}</p>
    <small>{actions.takeover.device}</small>
    <div className="actions"><button className="primary" disabled={!!actions.busy} onClick={() => void actions.takeOverAndContinue()}>{t("interaction.takeover")}</button><button disabled={!!actions.busy} onClick={actions.cancelTakeover}>{t("account.cancel")}</button></div>
  </div>;
  if (actions.pending) return <div className="operation-card" role="status">
    <strong>{actions.checking && <Spinner />}{t(actions.unresolved ? "experience.unresolvedTitle" : "experience.pendingTitle")}</strong>
    <p>{t(actions.unresolved ? "experience.unresolvedHelp" : "experience.pendingHelp")}</p>
    <div className="actions">
      <button disabled={actions.checking || !!actions.busy} onClick={() => void actions.checkOperation()}>{t(actions.checking ? "experience.checking" : "experience.pendingCheck")}</button>
      <button disabled={actions.pausing} onClick={() => void actions.perform(t("interaction.stopNetwork"), { action: "network-stop" })}>{actions.pausing && <Spinner />}{t("interaction.stopNetwork")}</button>
      {actions.unresolved && <button disabled={actions.checking || !!actions.busy} onClick={() => void actions.reviewPending()}>{t(actions.reviewed ? "experience.confirmReviewed" : "experience.reviewState")}</button>}
    </div>
  </div>;
  if (actions.busy || actions.pausing) return <Loading label={`${actions.pausing ? t("interaction.stopNetwork") : actions.busy}…`} help={t("experience.workingHelp")} />;
  return null;
}
