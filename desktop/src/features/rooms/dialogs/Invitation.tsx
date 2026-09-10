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
        {formatTime(dialog.invitation.expires_at)}{" "}
        前有效。新码生成后，旧码立即失效。
      </p>
      <button
        className="primary"
        onClick={() => void copy(dialog.invitation.code)}
        disabled={Date.parse(dialog.invitation.expires_at) <= Date.now()}
      >
        复制邀请码
      </button>
      <p className="hint">邀请码只在当前窗口内保留，不保存到本机历史记录。</p>
    </>
  );
}
