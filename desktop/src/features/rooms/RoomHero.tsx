import type { Status } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import type { RoomView } from "./use-room";
import { Art } from "../catalog/Artwork";
import { formatTime } from "../../shared/time";
import type { RoomResult } from "../../shared/model";
export function RoomHero({
  view,
  status,
  actions,
  usable,
}: {
  view: RoomView;
  status: Status;
  actions: Actions;
  usable: boolean;
}) {
  const { room, game, isCurrent, owner, canManage, members } = view;
  const { perform, setDialog, confirm } = actions;
  if (!room) return null;
  const count = members.length;
  const invite = () =>
    perform<RoomResult>(
      "生成邀请码",
      { action: "invite", room: room.id },
      (out) => {
        if (out.invitation)
          setDialog({
            type: "invite",
            room: out.room,
            invitation: out.invitation,
          });
      },
    );
  return (
    <section className="room-hero">
      {game && <div className="scene-art" aria-hidden="true"><Art game={game} kind="background" /></div>}
      <div className="hero-content">
        <div className="eyebrow">YOUR PARTY <span className="room-game-label">{room.game_name || room.game}</span></div>
        <h2>{room.name}</h2>
        <div className="hero-meta">
          <span className="pill">
            {isCurrent
              ? status.engine === "running"
                ? "网络运行中"
                : "网络已停止"
              : "仅管理"}
          </span>
          <span>
            {count} / {room.capacity} 位成员
          </span>
          <span>{formatTime(room.expires_at)} 到期</span>
        </div>
        <div className="actions">
          {owner && (
            <button
              className="primary"
              disabled={!usable || !canManage}
              onClick={() => void invite()}
            >
              生成新邀请码
            </button>
          )}
          {isCurrent && (
            <button
              disabled={!usable}
              onClick={() =>
                confirm(
                  "离开房间",
                  "停止本机联机。你是房主时，仍保留该房间的管理权。",
                  { action: "leave", room: room.id },
                )
              }
            >
              离开房间
            </button>
          )}
          {owner && (
            <button
              className="danger subtle"
              disabled={!usable || !canManage}
              onClick={() =>
                confirm("关闭房间", "所有成员将断开连接，这个房间无法恢复。", {
                  action: "close",
                  room: room.id,
                })
              }
            >
              关闭房间
            </button>
          )}
        </div>
      </div>
    </section>
  );
}
