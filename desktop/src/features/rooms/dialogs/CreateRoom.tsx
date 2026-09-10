import type { Actions } from "../../../app/use-actions";
import type { Dialog } from "./types";
import type { Status, RoomResult, Failure } from "../../../shared/model";
import { PortList } from "../../../shared/ui/PortList";
export function CreateRoom({
  dialog,
  actions,
  status,
  gamesError,
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
          "创建房间",
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
        房间名称
        <input
          autoFocus
          name="name"
          maxLength={40}
          required
          placeholder="给这场联机起个名字"
        />
      </label>
      <PortList ports={dialog.game.ports} />
      <p className="hint">
        每房最多 32 人，有效期 24 小时。游戏主机的实际监听端口须匹配配置。
      </p>
      <button
        className="primary"
        disabled={!!busy || !!gamesError || !!status?.selected_room}
      >
        创建并连接
      </button>
    </form>
  );
}
