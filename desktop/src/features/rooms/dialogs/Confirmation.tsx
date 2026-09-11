import { t } from "../../../i18n";
import type { Actions } from "../../../app/use-actions";
import type { Dialog } from "./types";
import type { Status, Failure } from "../../../shared/model";
export function Confirmation({
  dialog,
  actions,
  status,
  serviceError,
  onJoined,
}: {
  dialog: Extract<Dialog, { type: "confirm" }>;
  actions: Actions;
  status?: Status;
  gamesError?: Failure;
  serviceError?: Failure;
  onJoined: () => void;
}) {
  const { perform, busy, setDialog, quit } = actions;

  return (
    <>
      <p className="confirm-description">{dialog.description}</p>
      <div className="actions end">
        <button disabled={!!busy} onClick={() => setDialog(undefined)}>
          {t("confirmation.cancel")}</button>
        <button
          className="danger solid"
          disabled={!!busy || !!serviceError}
          onClick={async () => {
            const current = dialog;
            if (current.exit && !status?.selected_room) {
              await quit();
              return;
            }
            const ok = await perform(current.title, current.request, () => {
              setDialog(undefined);
              if (
                current.request.action === "close" ||
                current.request.action === "leave"
              )
                onJoined();
            });
            if (ok && current.exit) await quit();
          }}
        >
          {t("confirmation.action", { action: dialog.title })}
        </button>
      </div>
    </>
  );
}
