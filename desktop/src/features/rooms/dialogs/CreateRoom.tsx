import { t } from "../../../i18n";
import type { Actions } from "../../../app/use-actions";
import type { Dialog } from "./types";
import type { Status, RoomResult, Failure } from "../../../shared/model";
import { PortList } from "../../../shared/ui/PortList";
export function CreateRoom({
  dialog,
  actions,
  status,
  gamesError,
  serviceError,
  onJoined,
}: {
  dialog: Extract<Dialog, { type: "create" }>;
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
        void perform<RoomResult>(
          t("gameLibrary.createRoom"),
          {
            action: "create",
            body: {
              name: String(f.get("name")).trim(),
              game: dialog.game.id,
            },
          },
          (out) => {
            onJoined();
            if (out.invitation)
              setDialog({
                type: "invite",
                room: out.room,
                invitation: out.invitation,
              });
            else setDialog(undefined);
          },
        );
      }}
    >
      <p className="muted">{dialog.game.name}</p>
      <label>
        {t("createRoom.roomName")}<input
          autoFocus
          name="name"
          maxLength={40}
          required
          placeholder={t("createRoom.giveThisSessionAName")}
        />
      </label>
      <PortList ports={dialog.game.ports} />
      <p className="hint">
        {t("createRoom.limitsHelp")}</p>
      <button
        className="primary"
        disabled={!!busy || !!serviceError || !!gamesError || !!status?.selected_room}
      >
        {t("createRoom.createAndConnect")}</button>
    </form>
  );
}
