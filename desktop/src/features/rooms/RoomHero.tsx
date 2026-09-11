import { DotsThree, UserPlus, ArrowRight } from "@phosphor-icons/react";
import { t } from "../../i18n";
import type { Status, Invitation } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import type { ConnectionView, Recovery } from "../../app/experience";
import type { RoomView } from "./use-room";
import { Art } from "../catalog/Artwork";
import { formatTime } from "../../shared/time";

export function RoomHero({
  view,
  status,
  actions,
  usable,
  connection,
  onRecover,
}: {
  view: RoomView;
  status: Status;
  actions: Actions;
  usable: boolean;
  connection: ConnectionView;
  onRecover: (action: Recovery) => void;
}) {
  const { room, game, isCurrent, owner, canManage } = view;
  if (!room) return null;
  return (
    <section className="room-heading">
      <div className="room-heading-title">
        {game && (
          <div className="room-art small">
            <Art game={game} />
          </div>
        )}
        <div>
          <p>{room.game_name || room.game}</p>
          <h2>{room.name}</h2>
          <p className="hint">
            {t("roomHero.expires", { time: formatTime(room.expires_at) })}
          </p>
        </div>
      </div>
      <div className="room-heading-actions">
        {owner && (
          <button
            className="primary"
            disabled={!usable || !canManage}
            onClick={() =>
              void actions.perform<Invitation>(
                t("roomHero.generatingInviteCode"),
                {
                  action: "invite",
                  room: room.id,
                  body: { expected_revision: room.revision },
                },
                (invitation) =>
                  actions.setDialog({ type: "invite", room, invitation }),
              )
            }
          >
            <UserPlus size={19} aria-hidden="true" />
            {t("desk.inviteFriends")}
          </button>
        )}
        {isCurrent ? (
          <button
            disabled={!usable}
            onClick={() =>
              actions.confirm(
                t("roomHero.leaveRoom"),
                t("roomHero.leaveHelp"),
                { action: "leave", room: room.id },
              )
            }
          >
            {t("roomHero.leaveRoom")}
          </button>
        ) : (
          owner && (
            <button
              className="primary"
              disabled={
                !usable ||
                !canManage ||
                !!status.selected_room ||
                !!status.update?.required
              }
              onClick={() =>
                void actions.perform(t("interaction.rejoin"), {
                  action: "owner-join",
                  room: room.id,
                  body: { expected_revision: room.revision },
                })
              }
            >
              {t("desk.enterRoom")}
              <ArrowRight size={18} aria-hidden="true" />
            </button>
          )
        )}
        <details
          className="overflow-menu"
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.currentTarget.open = false;
              e.currentTarget.querySelector("summary")?.focus();
            }
          }}
        >
          <summary aria-label={t("desk.moreActions")}>
            <DotsThree size={26} aria-hidden="true" />
          </summary>
          <div>
            {isCurrent && (
              <>
                <button
                  disabled={!usable || !!status.update?.required}
                  onClick={() =>
                    void actions.perform(t("interaction.retryNetwork"), {
                      action: "network-retry",
                    })
                  }
                >
                  {t("interaction.retryNetwork")}
                </button>
              </>
            )}
            {owner && (
              <button
                className="danger"
                disabled={!usable || !canManage}
                onClick={() =>
                  actions.confirm(
                    t("roomHero.closeRoom"),
                    t("roomHero.closeHelp"),
                    {
                      action: "close",
                      room: room.id,
                      body: { expected_revision: room.revision },
                    },
                  )
                }
              >
                {t("roomHero.closeRoom")}
              </button>
            )}
          </div>
        </details>
      </div>
      <div
        className="connection-state"
        data-tone={isCurrent ? connection.tone : "neutral"}
        role="status"
      >
        <strong>
          {t(isCurrent ? connection.title : "roomHero.manageOnly")}
        </strong>
        <p>{t(isCurrent ? connection.help : "desk.manageOnlyHelp")}</p>
        {isCurrent && connection.recovery && !connection.ready && (
          <button onClick={() => onRecover(connection.recovery!)}>
            {t("desk.resolveConnection")}
          </button>
        )}
      </div>
    </section>
  );
}
