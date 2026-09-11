import { failure } from "../../../native/api";
import { t } from "../../../i18n";
import { Modal } from "../../../shared/ui/Modal";
import type { Actions } from "../../../app/use-actions";
import type { Status, Failure } from "../../../shared/model";
import { CreateRoom } from "./CreateRoom";
import { JoinRoom } from "./JoinRoom";
import { Invitation } from "./Invitation";
import { Confirmation } from "./Confirmation";

export function RoomDialogs(props: {
  actions: Actions;
  status: Status;
  gamesError?: Failure;
  serviceError?: Failure;
  onJoined: () => void;
}) {
  const { actions } = props;
  const { dialog, busy, error, setDialog, setError } = actions;
  if (!dialog) return null;
  const title =
    dialog.type === "create"
      ? t("gameLibrary.createRoom")
      : dialog.type === "join"
        ? t("roomDialogs.joinWithAnInviteCode")
        : dialog.type === "invite"
          ? t("roomDialogs.inviteFriendsToPlay")
          : dialog.title;
  return (
    <Modal
      title={title}
      busy={!!busy}
      onClose={() => {
        setDialog(undefined);
        setError(undefined);
      }}
    >
      {error && (
        <div className="banner error" role="alert">
          {failure(error).error}
          {error.request_id && (
            <small className="selectable">{error.request_id}</small>
          )}
        </div>
      )}
      {actions.takeover && (
        <div className="banner warning" role="alert">
          <p>{t("interaction.takeoverHelp")}</p>
          <small>{actions.takeover.device}</small>
          <button
            disabled={!!busy}
            onClick={() => void actions.takeOverAndContinue()}
          >
            {t("interaction.takeover")}
          </button>
          <button onClick={actions.cancelTakeover}>
            {t("account.cancel")}
          </button>
        </div>
      )}
      {actions.pending && (
        <div className="banner warning" role="status">
          <p>{t("interaction.pending")}</p>
          <button onClick={() => void actions.checkOperation(actions.pending!)}>
            {t("interaction.checkOperation")}
          </button>
          <button
            onClick={() =>
              void actions.perform(t("interaction.stopNetwork"), {
                action: "network-stop",
              })
            }
          >
            {t("interaction.stopNetwork")}
          </button>
        </div>
      )}
      {dialog.type === "create" && <CreateRoom {...props} dialog={dialog} />}
      {dialog.type === "join" && <JoinRoom {...props} dialog={dialog} />}
      {dialog.type === "invite" && <Invitation {...props} dialog={dialog} />}
      {dialog.type === "confirm" && <Confirmation {...props} dialog={dialog} />}
    </Modal>
  );
}
