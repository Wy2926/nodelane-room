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
      ? "创建房间"
      : dialog.type === "join"
        ? "通过邀请码加入"
        : dialog.type === "invite"
          ? "邀请朋友一起玩"
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
          {error.error}
        </div>
      )}
      {dialog.type === "create" && <CreateRoom {...props} dialog={dialog} />}
      {dialog.type === "join" && <JoinRoom {...props} dialog={dialog} />}
      {dialog.type === "invite" && <Invitation {...props} dialog={dialog} />}
      {dialog.type === "confirm" && <Confirmation {...props} dialog={dialog} />}
    </Modal>
  );
}
