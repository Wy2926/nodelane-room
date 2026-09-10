import type { Actions } from "../../../app/use-actions";
import type { Dialog } from "./types";
import type { Status, Failure } from "../../../shared/model";
export function JoinRoom({
  actions,
  status,
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
          "加入房间",
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
      <p className="muted">使用朋友发来的邀请码，加入同一个游戏房间。</p>
      <label>
        邀请码
        <input
          autoFocus
          autoComplete="off"
          spellCheck={false}
          name="code"
          required
          maxLength={128}
          placeholder="粘贴邀请码"
        />
      </label>
      <button className="primary" disabled={!!busy || !!status?.selected_room}>
        加入并连接
      </button>
    </form>
  );
}
