import { t } from "../../../i18n";
import type { Actions } from "../../../app/use-actions";
import type { Dialog } from "./types";
import type { Status, Failure } from "../../../shared/model";
import { formatTime } from "../../../shared/time";
export function Invitation({
  dialog,
  actions,
}: {
  dialog: Extract<Dialog, { type: "invite" }>;
  actions: Actions;
  status?: Status;
  gamesError?: Failure;
  serviceError?: Failure;
  onJoined: () => void;
}) {
  const { copy } = actions;

  return (
    <>
      <p>
        {dialog.room.name} · {dialog.room.game_name}
      </p>
      <div className="invitation selectable">{dialog.invitation.code}</div>
      <p className="muted">
        {t("invitation.validUntil", { time: formatTime(dialog.invitation.expires_at) })}</p>
      <button
        className="primary"
        onClick={() => void copy(dialog.invitation.code)}
        disabled={Date.parse(dialog.invitation.expires_at) <= Date.now()}
      >
        {t("invitation.copyInviteCode")}</button>
      <p className="hint">{t("invitation.storageHelp")}</p>
    </>
  );
}
