import { t } from "../../../i18n";
import { Modal } from "../../../shared/ui/Modal";
import type { Actions } from "../../../app/use-actions";
import type { Status, Failure, Game } from "../../../shared/model";
import type { Recovery } from "../../../app/experience";
import { CreateRoom } from "./CreateRoom";
import { Problem } from "../../../app/Problem";
import { Invitation } from "./Invitation";
import { Confirmation } from "./Confirmation";

export function RoomDialogs(props: {
  actions: Actions;
  status: Status;
  gamesError?: Failure;
  games?: Game[];
  onRecover?: (action: Recovery) => void;
  serviceError?: Failure;
  onJoined: () => void;
}) {
  const { actions } = props;
  const { dialog, busy, error, setDialog, setError } = actions;
  if (!dialog) return null;
  const title =
    dialog.type === "create"
      ? t("gameLibrary.createRoom")
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
        <Problem
          error={error}
          onRecover={props.onRecover}
          onDismiss={() => setError(undefined)}
        />
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
      {dialog.type === "invite" && <Invitation {...props} dialog={dialog} />}
      {dialog.type === "confirm" && <Confirmation {...props} dialog={dialog} />}
    </Modal>
  );
}
