import { t } from "../../../i18n";
import { failure } from "../../../native/api";
import type { Actions } from "../../../app/use-actions";
import type { Status, RoomResult, Failure, Game } from "../../../shared/model";
import { Art } from "../../catalog/Artwork";

export function CreateRoom({
  actions,
  status,
  games = [],
  gamesError,
  serviceError,
  onJoined,
}: {
  actions: Actions;
  status?: Status;
  games?: Game[];
  gamesError?: Failure;
  serviceError?: Failure;
  onJoined: () => void;
}) {
  const game = games.find((game) => game.id === "custom" && game.enabled);
  const allowed =
    status?.room_creation?.allowed === true &&
    !actions.busy &&
    !serviceError &&
    !gamesError &&
    status.update?.state !== "installing" &&
    !status.selected_room &&
    !status.update?.required &&
    status.control === "online";
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (!allowed || !game) return;
        const name = String(new FormData(e.currentTarget).get("name")).trim();
        if (!name) return;
        if (new TextEncoder().encode(name).length > 120) {
          actions.setError({
            code: "request_validation_failed",
            error: t("interaction.byteLimit"),
          });
          return;
        }
        void actions.perform<RoomResult>(
          t("gameLibrary.createRoom"),
          {
            action: "create",
            body: {
              name,
              game: game.id,
              expected_game_revision: game.revision,
            },
          },
          (out) => {
            onJoined();
            actions.setDialog(
              out.invitation
                ? { type: "invite", room: out.room, invitation: out.invitation }
                : undefined,
            );
          },
        );
      }}
    >
      {game && (
        <div className="create-room-cover">
          <div className="room-art small">
            <Art game={game} />
          </div>
          <div>
            <strong>{t("createRoom.genericRoom")}</strong>
            <p className="hint">{t("createRoom.genericHelp")}</p>
          </div>
        </div>
      )}
      <label>
        {t("createRoom.roomName")}
        <input
          name="name"
          autoFocus
          maxLength={120}
          required
          placeholder={t("createRoom.giveThisSessionAName")}
          disabled={!allowed || !game}
        />
      </label>
      {!game && (
        <p className="hint" role="status">
          {t("createRoom.unavailable")}
        </p>
      )}
      {status?.room_creation?.reason && (
        <p className="hint" role="status">
          {failure({ code: status.room_creation.reason }).error}
        </p>
      )}
      <button className="primary full" disabled={!allowed || !game}>
        {t("createRoom.createAndConnect")}
      </button>
    </form>
  );
}
