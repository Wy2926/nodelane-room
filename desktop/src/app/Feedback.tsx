import { failure } from "../native/api";
import { t } from "../i18n";
import type { useService } from "../native/use-service";
import type { Actions } from "./use-actions";
import { formatTime } from "../shared/time";

export function Feedback({
  service,
  actions,
  refreshAll,
}: {
  service: ReturnType<typeof useService>;
  actions: Actions;
  refreshAll: () => void;
}) {
  const { status, error: serviceError, refresh } = service;
  const { dialog, error, notice, busy } = actions;
  return (
    <>
      {serviceError && (
        <div className="banner error" role="alert">
          <div>
            <strong>{t("feedback.cannotConnectToTheNetworkService")}</strong>
            <p>{failure(serviceError).error}</p>
          </div>
          <div className="actions"><button onClick={refresh}>{t("feedback.checkAgain")}</button><button onClick={() => void actions.quit()}>{t("feedback.exitApp")}</button></div>
        </div>
      )}
      {status?.error && (
        <div className="banner warning" role="status">
          <div>
            <strong>
              {status.control === "unreachable"
                ? t("feedback.controlUnavailable")
                : t("feedback.networkNeedsAttention")}
            </strong>
            <p>{status.error}</p>
            {status.lease_expires_at && (
              <small>{t("feedback.authorizationExpires")}{formatTime(status.lease_expires_at)}</small>
            )}
          </div>
          <button onClick={refreshAll}>{t("feedback.refreshStatus")}</button>
        </div>
      )}
      {!dialog && error && (
        <div className="banner error" role="alert">
          {failure(error).error}
        </div>
      )}
      {notice && (
        <div className="toast" role="status">
          {t("useActions.copied")}
        </div>
      )}
      {busy && (
        <div className="working" role="status">
          {busy}…
        </div>
      )}
    </>
  );
}
