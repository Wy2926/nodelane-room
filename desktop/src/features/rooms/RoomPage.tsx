import { failure } from "../../native/api";
import { t } from "../../i18n";
import type { Status } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import type { RoomView } from "./use-room";
import { Empty } from "../../shared/ui/Empty";
import type { Catalog } from "../catalog/use-catalog";
import type { Page } from "../../app/navigation";
import { RoomHero } from "./RoomHero";
import { Members } from "./Members";
import { Connection } from "./Connection";
import { Art } from "../catalog/Artwork";
import {
  ArrowRight,
  Check,
  GameController,
  Ticket,
} from "@phosphor-icons/react";
export function RoomPage({
  view,
  status,
  actions,
  usable,
  catalog,
  setPage,
  refreshAll,
}: {
  view: RoomView;
  status: Status;
  actions: Actions;
  usable: boolean;
  catalog: Catalog;
  setPage: (page: Page) => void;
  refreshAll: () => void;
}) {
  const {
    room,
    game,
    roomFresh,
    selected,
    setSelected,
    activeRoom,
    managementError,
  } = view;
  const { rooms, roomsError } = catalog;
  const { setDialog, setError } = actions;
  const currentCard =
    activeRoom && !rooms.some((r) => r.id === activeRoom.id)
      ? [activeRoom, ...rooms]
      : rooms;
  return (
    <>
      {(!!room || !!status.selected_room) && (
        <div className="section-toolbar room-toolbar">
          <span className="eyebrow">{t("roomPage.yourRooms")}</span>
          {!status.selected_room && (
            <div className="actions">
              <button
                onClick={() => {
                  setError(undefined);
                  setDialog({ type: "join" });
                }}
                disabled={!usable || !!status.selected_room}
              >
                {t("roomPage.joinWithInviteCode")}
              </button>
              <button
                className="primary"
                onClick={() => setPage("games")}
                disabled={!usable || !!status.selected_room}
              >
                {t("gameLibrary.createRoom")}
                <span aria-hidden="true">＋</span>
              </button>
            </div>
          )}
        </div>
      )}
      {roomsError && (
        <div className="banner warning" role="alert">
          {t("roomPage.roomListCouldNotBeRefreshed")}
          {failure(roomsError).error}
          <button onClick={refreshAll}>{t("gameLibrary.retry")}</button>
        </div>
      )}
      {catalog.truncated && (
        <p role="status">{t("interaction.roomsTruncated")}</p>
      )}
      {currentCard.length > 0 && (
        <div className="room-tabs" aria-label={t("roomPage.chooseARoom")}>
          {currentCard.map((r) => {
            const roomGame = catalog.games.find((g) => g.id === r.game);
            return (
              <button
                key={r.id}
                aria-pressed={room?.id === r.id}
                onClick={() => {
                  setSelected(r.id);
                  setError(undefined);
                }}
              >
                <span className="room-tab-cover">
                  {roomGame ? (
                    <Art game={roomGame} />
                  ) : (
                    <GameController
                      size={24}
                      weight="light"
                      aria-hidden="true"
                    />
                  )}
                </span>
                <span className="room-tab-copy">
                  <strong>{r.name}</strong>
                  <small>{r.game_name || r.game}</small>
                </span>
                <span className="room-tab-state">
                  {room?.id === r.id && <Check size={14} aria-hidden="true" />}
                  {r.id === activeRoom?.id
                    ? t("roomPage.joined")
                    : t("roomHero.manageOnly")}
                </span>
              </button>
            );
          })}
        </div>
      )}
      {managementError && (
        <div className="banner error" role="alert">
          {failure(managementError).error}
          <button onClick={refreshAll}>{t("roomPage.reload")}</button>
        </div>
      )}
      {room ? (
        <>
          <RoomHero
            view={view}
            status={status}
            actions={actions}
            usable={usable}
          />
          {!roomFresh && (
            <div className="banner warning" role="status">
              {t("roomPage.staleHelp")}
              <button onClick={refreshAll}>{t("roomPage.refresh")}</button>
            </div>
          )}
          {game?.enabled === false && (
            <div className="banner warning" role="status">
              {t("roomPage.disabledGameHelp")}
            </div>
          )}
          <div className="room-columns">
            <Members
              view={view}
              status={status}
              actions={actions}
              usable={usable}
            />
            <Connection view={view} status={status} />
          </div>
        </>
      ) : (
        !selected &&
        (status.selected_room ? (
          <Empty title={t("roomPage.syncingRoom")}>
            <p>{t("roomPage.syncingHelp")}</p>
          </Empty>
        ) : (
          <section className="room-welcome">
            <div className="welcome-copy">
              <span className="eyebrow">
                {t("roomPage.yourNextGameStartsTogether")}
              </span>
              <h2>
                {t("roomPage.whereverYouAre")}
                <br />
                {t("roomPage.shareTheSameRoom")}
                <span>{t("roomPage.punctuation")}</span>
              </h2>
              <p>
                {t("roomPage.createYourOwnWorldOrJoinAFriend")}
                <br />
                {t("roomPage.tonightTheChoiceIsYours")}
              </p>
            </div>
            <div className="welcome-cards">
              <button
                className="launch-card launch-create"
                disabled={!usable}
                onClick={() => setPage("games")}
              >
                <span className="launch-icon">
                  <GameController size={32} weight="light" aria-hidden="true" />
                </span>
                <span className="launch-copy">
                  <strong>{t("gameLibrary.createRoom")}</strong>
                  <small>{t("roomPage.pickAGameAndInviteYourFriends")}</small>
                </span>
                <ArrowRight size={23} aria-hidden="true" />
              </button>
              <button
                className="launch-card"
                disabled={!usable}
                onClick={() => {
                  setError(undefined);
                  setDialog({ type: "join" });
                }}
              >
                <span className="launch-icon">
                  <Ticket size={32} weight="light" aria-hidden="true" />
                </span>
                <span className="launch-copy">
                  <strong>{t("roomPage.joinWithInviteCode")}</strong>
                  <small>{t("roomPage.joinHelp")}</small>
                </span>
                <ArrowRight size={23} aria-hidden="true" />
              </button>
            </div>
          </section>
        ))
      )}
    </>
  );
}
