import { t } from "../../../i18n";
import type { Actions } from "../../../app/use-actions";
import type { Dialog } from "./types";
import type { Status, Failure } from "../../../shared/model";
export function JoinRoom({
  actions,
  status,
  serviceError,
  onJoined,
}: {
  dialog: Extract<Dialog, { type: "join" }>;
  actions: Actions;
  status?: Status;
  gamesError?: Failure;
  serviceError?: Failure;
  onJoined: () => void;
}) {
  const { perform, busy, setDialog } = actions;

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        const f = new FormData(e.currentTarget);
        void perform(
          t("joinRoom.joiningRoom"),
          {
            action: "join",
            body: { code: String(f.get("code")).trim() },
          },
          () => {
            setDialog(undefined);
            onJoined();
          },
        );
      }}
    >
      <p className="muted">{t("joinRoom.useYourFriendSInviteCodeToJoin")}</p>
      <label>
        {t("joinRoom.inviteCode")}<input
          autoFocus
          autoComplete="off"
          spellCheck={false}
          name="code"
          required
          maxLength={128}
          placeholder={t("joinRoom.pasteInviteCode")}
        />
      </label>
      <button className="primary" disabled={!!busy || !!serviceError || !!status?.selected_room}>
        {t("joinRoom.joinAndConnect")}</button>
    </form>
  );
}
