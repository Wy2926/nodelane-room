import { useEffect, useRef, useState } from "react";
import { ArrowLeft, ArrowRight, Users, Info } from "@phosphor-icons/react";
import { failure } from "../../native/api";
import { t } from "../../i18n";
import type { Status, Room } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import type { RoomView } from "./use-room";
import type { Catalog } from "../catalog/use-catalog";
import type { ConnectionView, Recovery } from "../../app/experience";
import { RoomHero } from "./RoomHero";
import { Members } from "./Members";
import { Connection } from "./Connection";
import { Art } from "../catalog/Artwork";
import { Empty } from "../../shared/ui/Empty";

export function RoomPage({
  view,
  status,
  actions,
  usable,
  catalog,
  refreshAll,
  connection,
  onRecover,
}: {
  view: RoomView;
  status: Status;
  actions: Actions;
  usable: boolean;
  catalog: Catalog;
  refreshAll: () => void;
  connection: ConnectionView;
  onRecover: (action: Recovery) => void;
}) {
  const [details, setDetails] = useState(!!status.selected_room);
  const [ownedOnly, setOwnedOnly] = useState(false);
  const workspace = useRef<HTMLDivElement>(null);
  useEffect(() => {
    setDetails(!!status.selected_room);
  }, [status.selected_room]);
  const { room, activeRoom, game, roomFresh, managementError, selected } = view;
  useEffect(() => {
    const content = workspace.current?.closest("main");
    if (content) content.scrollTop = 0;
  }, [details, selected, status.selected_room]);
  const rooms = activeRoom
    ? [activeRoom, ...catalog.rooms.filter((r) => r.id !== activeRoom.id)]
    : catalog.rooms;
  const visible = rooms.filter(
    (r) => !ownedOnly || r.owner_user_id === status.user?.id,
  );
  const joiningAllowed =
    usable &&
    !status.selected_room &&
    !status.update?.required &&
    status.update?.state !== "installing";
  const genericAvailable = catalog.games.some(
    (game) => game.id === "custom" && game.enabled,
  );
  const creationAllowed =
    joiningAllowed &&
    status.room_creation?.allowed === true &&
    !catalog.gamesError &&
    !catalog.loading &&
    genericAvailable;
  const restriction = !status.room_creation?.allowed
    ? status.room_creation?.reason
    : undefined;
  const open = async (target: Room) => {
    actions.setError(undefined);
    view.setSelected(target.id);
    setDetails(true);
    if (joiningAllowed && target.owner_user_id === status.user?.id)
      await actions.perform(
        t("desk.enterRoom"),
        {
          action: "owner-join",
          room: target.id,
          body: { expected_revision: target.revision },
        },
        () => view.setSelected(""),
      );
  };
  return (
    <div
      ref={workspace}
      className={`room-workspace${details ? " room-detail" : ""}`}
    >
      <section className="room-main">
        {details ? (
          <>
            <button
              className="text-button back-link"
              onClick={() => setDetails(false)}
            >
              <ArrowLeft size={18} aria-hidden="true" />
              {t("desk.backToRooms")}
            </button>
            {room ? (
              <>
                {!roomFresh && (
                  <div className="banner warning" role="status">
                    {t("roomPage.staleHelp")}
                    <button onClick={refreshAll}>
                      {t("roomPage.refresh")}
                    </button>
                  </div>
                )}
                {game?.enabled === false && (
                  <div className="banner warning">
                    {t("roomPage.disabledGameHelp")}
                  </div>
                )}
                <Members {...{ view, status, actions, usable }} />
              </>
            ) : (
              <Empty title={t("roomPage.syncingRoom")}>
                <p>{t("roomPage.syncingHelp")}</p>
              </Empty>
            )}
            {managementError && (
              <div className="banner error" role="alert">
                {failure(managementError).error}
                <button onClick={refreshAll}>{t("roomPage.reload")}</button>
              </div>
            )}
          </>
        ) : (
          <>
            <div className="page-intro">
              <h2>{t("navigation.myRooms")}</h2>
              <p>{t("desk.roomsIntro")}</p>
            </div>
            <div
              className="room-filters"
              role="group"
              aria-label={t("desk.roomFilter")}
            >
              <button
                aria-pressed={!ownedOnly}
                onClick={() => setOwnedOnly(false)}
              >
                {t("desk.allRooms")}
              </button>
              <button
                aria-pressed={ownedOnly}
                onClick={() => setOwnedOnly(true)}
              >
                {t("desk.ownedRooms")}
              </button>
            </div>
            {catalog.loading ? (
              <Empty title={t("desk.loadingRooms")} />
            ) : (
              <div className="room-list">
                {visible.map((r) => {
                  const artwork = catalog.games.find((g) => g.id === r.game);
                  return (
                    <article
                      className="room-row"
                      data-current={activeRoom?.id === r.id}
                      key={r.id}
                    >
                      <div className="room-art">
                        <Art game={artwork || { id: r.game, cover_url: "" }} />
                      </div>
                      <div className="room-row-copy">
                        {activeRoom?.id === r.id && (
                          <span className="room-current-label">
                            {t("roomPage.joined")}
                          </span>
                        )}
                        <h3>{r.name}</h3>
                        <p>{r.game_name || r.game}</p>
                        <span className="room-capacity">
                          <Users size={18} weight="fill" aria-hidden="true" />
                          {activeRoom?.id === r.id
                            ? `${status.members.length} / ${r.capacity}`
                            : t("desk.capacity", { count: r.capacity })}
                        </span>
                      </div>
                      <button
                        className="room-enter text-button"
                        disabled={!usable}
                        onClick={() => void open(r)}
                      >
                        {t(joiningAllowed ? "desk.enterRoom" : "desk.viewRoom")}
                        <ArrowRight size={24} aria-hidden="true" />
                      </button>
                    </article>
                  );
                })}
                {!visible.length && (
                  <Empty title={t("desk.noRooms")}>
                    <p>{t("desk.noRoomsHelp")}</p>
                  </Empty>
                )}
              </div>
            )}
            {catalog.roomsError && (
              <div className="banner warning" role="alert">
                {t("roomPage.roomListCouldNotBeRefreshed")}{" "}
                {failure(catalog.roomsError).error}
                <button onClick={refreshAll}>{t("roomPage.refresh")}</button>
              </div>
            )}
            {catalog.truncated && (
              <p className="hint">{t("interaction.roomsTruncated")}</p>
            )}
          </>
        )}
      </section>
      <aside
        className="room-join"
        aria-label={t(details ? "desk.roomInformation" : "desk.roomActions")}
      >
        {details ? (
          <>
            {room && (
              <div className="room-cover">
                <Art game={game || { id: room.game, cover_url: "" }} />
              </div>
            )}
            <div className="room-sidebar-content">
              <RoomHero
                {...{ view, status, actions, usable, connection, onRecover }}
              />
              <Connection {...{ view, status }} />
              {selected && (
                <div className="context-note">
                  <Info size={18} aria-hidden="true" />
                  <p>{t("desk.roomContext")}</p>
                </div>
              )}
            </div>
          </>
        ) : (
          <>
            <section>
              <h2>{t("desk.haveInvite")}</h2>
              <p>{t("desk.joinIntro")}</p>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  if (!joiningAllowed) return;
                  const form = e.currentTarget;
                  const code = String(new FormData(form).get("code")).trim();
                  if (!code) return;
                  void actions.perform(
                    t("joinRoom.joinAndConnect"),
                    { action: "join", body: { code } },
                    () => {
                      form.reset();
                      view.setSelected("");
                      setDetails(true);
                    },
                  );
                }}
              >
                <label>
                  {t("joinRoom.inviteCode")}
                  <input
                    name="code"
                    required
                    maxLength={128}
                    autoComplete="off"
                    spellCheck={false}
                    placeholder={t("joinRoom.pasteInviteCode")}
                    disabled={!joiningAllowed}
                  />
                </label>
                <button className="primary full" disabled={!joiningAllowed}>
                  {t("desk.joinRoom")}
                </button>
              </form>
              {status.selected_room && (
                <p className="hint">{t("desk.leaveFirst")}</p>
              )}
              {status.update?.required && (
                <p className="hint">{t("experience.actionsUpdate")}</p>
              )}
            </section>
            <section className="create-entry">
              <h2>{t("gameLibrary.createRoom")}</h2>
              <button
                className="primary full"
                disabled={!creationAllowed}
                aria-describedby="creation-reason"
                onClick={() => {
                  actions.setError(undefined);
                  actions.setDialog({ type: "create" });
                }}
              >
                {t("gameLibrary.createRoom")}
              </button>
              <p id="creation-reason" className="hint">
                {restriction === "account_disabled" ? (
                  <>
                    {t("desk.creationRestricted")}
                    <br />
                    {t("desk.creationRestrictedHelp")}
                  </>
                ) : restriction ? (
                  failure({ code: restriction }).error
                ) : status.selected_room ? (
                  t("desk.leaveFirst")
                ) : !status.room_creation?.allowed ? (
                  t("desk.checkingPermission")
                ) : !catalog.loading &&
                  !catalog.gamesError &&
                  !genericAvailable ? (
                  t("createRoom.unavailable")
                ) : (
                  t("desk.createIntro")
                )}
              </p>
              {!genericAvailable && !catalog.loading && !catalog.gamesError && (
                <button className="text-button" onClick={refreshAll}>
                  {t("roomPage.refresh")}
                </button>
              )}
              {restriction && (
                <details className="restriction-details">
                  <summary>
                    {t("desk.restrictionDetails")}
                    <ArrowRight size={18} aria-hidden="true" />
                  </summary>
                  <p>{failure({ code: restriction }).error}</p>
                  <button
                    className="text-button"
                    onClick={
                      restriction === "client_update_required"
                        ? () => onRecover("updates")
                        : refreshAll
                    }
                  >
                    {t(
                      restriction === "client_update_required"
                        ? "experience.openUpdates"
                        : "roomPage.refresh",
                    )}
                  </button>
                </details>
              )}
              {catalog.gamesError && (
                <p className="hint">
                  {failure(catalog.gamesError).error}
                  <button className="text-button" onClick={refreshAll}>
                    {t("gameLibrary.retry")}
                  </button>
                </p>
              )}
            </section>
          </>
        )}
      </aside>
    </div>
  );
}
