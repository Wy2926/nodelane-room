import { t } from "../../i18n";
import type { Status } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import type { RoomView } from "./use-room";
import { Art } from "../catalog/Artwork";
import { formatTime } from "../../shared/time";
import type { Invitation } from "../../shared/model";
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
    perform<Invitation>(
      t("roomHero.generatingInviteCode"),
      {
        action: "invite",
        room: room.id,
        body: { expected_revision: room.revision },
      },
      (invitation) => setDialog({ type: "invite", room, invitation }),
    );
  return (
    <section className="room-hero">
      {game && (
        <div className="scene-art" aria-hidden="true">
          <Art game={game} kind="background" />
        </div>
      )}
      <div className="hero-content">
        <div className="eyebrow">
          {t("members.yourParty")}
          <span className="room-game-label">{room.game_name || room.game}</span>
        </div>
        <h2>{room.name}</h2>
        <div className="hero-meta">
          <span className="pill">
            {isCurrent
              ? status.network?.state === "ready"
                ? t("roomHero.networkRunning")
                : t("roomHero.networkStopped")
              : t("roomHero.manageOnly")}
          </span>
          <span>
            {t("roomHero.memberCount", { count, capacity: room.capacity })}
          </span>
          <span>
            {t("roomHero.expires", { time: formatTime(room.expires_at) })}
          </span>
        </div>
        <div className="actions">
          {owner && !isCurrent && (
            <button
              disabled={!usable || !canManage || !!status.selected_room}
              onClick={() =>
                void perform(t("interaction.rejoin"), {
                  action: "owner-join",
                  room: room.id,
                  body: { expected_revision: room.revision },
                })
              }
            >
              {t("interaction.rejoin")}
            </button>
          )}
          {isCurrent && (
            <button
              disabled={!usable}
              onClick={() =>
                void perform(t("interaction.retryNetwork"), {
                  action: "network-retry",
                })
              }
            >
              {t("interaction.retryNetwork")}
            </button>
          )}
          {isCurrent && (
            <button
              onClick={() =>
                void perform(t("interaction.stopNetwork"), {
                  action: "network-stop",
                })
              }
            >
              {t("interaction.stopNetwork")}
            </button>
          )}
          {owner && (
            <button
              className="primary"
              disabled={!usable || !canManage}
              onClick={() => void invite()}
            >
              {t("roomHero.generateNewInviteCode")}
            </button>
          )}
          {isCurrent && (
            <button
              disabled={!usable}
              onClick={() =>
                confirm(t("roomHero.leaveRoom"), t("roomHero.leaveHelp"), {
                  action: "leave",
                  room: room.id,
                })
              }
            >
              {t("roomHero.leaveRoom")}
            </button>
          )}
          {owner && (
            <button
              className="danger subtle"
              disabled={!usable || !canManage}
              onClick={() =>
                confirm(t("roomHero.closeRoom"), t("roomHero.closeHelp"), {
                  action: "close",
                  room: room.id,
                  body: { expected_revision: room.revision },
                })
              }
            >
              {t("roomHero.closeRoom")}
            </button>
          )}
        </div>
      </div>
    </section>
  );
}
