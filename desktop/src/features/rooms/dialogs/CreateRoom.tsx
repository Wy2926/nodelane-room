import { useState } from "react";
import { t } from "../../../i18n";
import { failure } from "../../../native/api";
import type { Actions } from "../../../app/use-actions";
import type { Dialog } from "./types";
import type { Status, RoomResult, Failure, Game } from "../../../shared/model";
import { Art } from "../../catalog/Artwork";

export function CreateRoom({
  dialog,
  actions,
  status,
  games = [],
  gamesError,
  serviceError,
  onJoined,
}: {
  dialog: Extract<Dialog, { type: "create" }>;
  actions: Actions;
  status?: Status;
  games?: Game[];
  gamesError?: Failure;
  serviceError?: Failure;
  onJoined: () => void;
}) {
  const [selected, setSelected] = useState(dialog.game?.id || "");
  const available = games.filter((game) => game.enabled);
  const game =
    available.find((game) => game.id === selected) ||
    available[0] ||
    dialog.game;
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
      <label>
        {t("desk.chooseGame")}
        <select
          autoFocus
          value={game?.id || ""}
          onChange={(e) => setSelected(e.target.value)}
          disabled={!allowed || !!actions.busy}
        >
          {available.map((game) => (
            <option value={game.id} key={game.id}>
              {game.name}
            </option>
          ))}
        </select>
      </label>
      {game && (
        <div className="selected-game">
          <div className="room-art small">
            <Art game={game} />
          </div>
          <div>
            <strong>{game.name}</strong>
            <p className="hint">{t("createRoom.limitsHelp")}</p>
          </div>
        </div>
      )}
      <label>
        {t("createRoom.roomName")}
        <input
          name="name"
          maxLength={120}
          required
          placeholder={t("createRoom.giveThisSessionAName")}
          disabled={!allowed || !!actions.busy}
        />
      </label>
      {status?.room_creation?.reason && (
        <p className="hint" role="status">
          {failure({ code: status.room_creation.reason }).error}
        </p>
      )}
      <button
        className="primary full"
        disabled={
          !allowed || !game || !!actions.busy || !!serviceError || !!gamesError
        }
      >
        {t("createRoom.createAndConnect")}
      </button>
    </form>
  );
}
