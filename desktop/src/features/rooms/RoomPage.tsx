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
import { ArrowRight, Check, GameController, Ticket } from "@phosphor-icons/react";
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
      {(!!room || !!status.selected_room) && <div className="section-toolbar room-toolbar">
        <span className="eyebrow">YOUR ROOMS</span>
        {!status.selected_room && <div className="actions">
          <button
            onClick={() => {
              setError(undefined);
              setDialog({ type: "join" });
            }}
            disabled={!usable || !!status.selected_room}
          >
            邀请码入房
          </button>
          <button
            className="primary"
            onClick={() => setPage("games")}
            disabled={!usable || !!status.selected_room}
          >
            创建房间 <span aria-hidden="true">＋</span>
          </button>
        </div>}
      </div>}
      {roomsError && (
        <div className="banner warning" role="alert">
          房间列表暂未更新：{roomsError.error}
          <button onClick={refreshAll}>重试</button>
        </div>
      )}
      {currentCard.length > 0 && (
        <div className="room-tabs" aria-label="选择房间">
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
              <span className="room-tab-cover">{roomGame ? <Art game={roomGame} /> : <GameController size={24} weight="light" aria-hidden="true" />}</span>
              <span className="room-tab-copy"><strong>{r.name}</strong><small>{r.game_name || r.game}</small></span>
              <span className="room-tab-state">
                {room?.id === r.id && <Check size={14} aria-hidden="true" />}
                {r.id === activeRoom?.id ? "已加入" : "仅管理"}
              </span>
            </button>
          );})}
        </div>
      )}
      {managementError && (
        <div className="banner error" role="alert">
          {managementError.error}
          <button onClick={refreshAll}>重新读取</button>
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
              房间状态尚未同步或已陈旧，显示的是最近一次数据。
              <button onClick={refreshAll}>刷新</button>
            </div>
          )}
          {game?.enabled === false && (
            <div className="banner warning" role="status">
              此游戏已被管理员停用，禁止新成员加入；游戏端口按最新授权撤回，房间管理和诊断仍可用。
            </div>
          )}
          <div className="room-columns">
            <Members
              view={view}
              status={status}
              actions={actions}
              usable={usable}
            />
            <Connection
              view={view}
              status={status}
              actions={actions}
              usable={usable}
            />
          </div>
        </>
      ) : (
        !selected && (
          status.selected_room ? <Empty title="正在同步房间"><p>正在获取房间授权与连接状态。</p></Empty> : <section className="room-welcome">
            <div className="welcome-copy"><span className="eyebrow">一起，开启下一局</span><h2>距离再远，<br />也在同一个房间<span>。</span></h2><p>创建你们的世界，或加入朋友的冒险。<br />今晚的主场，由你们决定。</p></div>
            <div className="welcome-cards">
              <button className="launch-card launch-create" disabled={!usable} onClick={() => setPage("games")}><span className="launch-icon"><GameController size={32} weight="light" aria-hidden="true" /></span><span className="launch-copy"><strong>创建房间</strong><small>选择一款游戏，邀请朋友一起玩</small></span><ArrowRight size={23} aria-hidden="true" /></button>
              <button className="launch-card" disabled={!usable} onClick={() => { setError(undefined); setDialog({ type: "join" }); }}><span className="launch-icon"><Ticket size={32} weight="light" aria-hidden="true" /></span><span className="launch-copy"><strong>邀请码入房</strong><small>朋友已经开好房间？输入邀请码加入</small></span><ArrowRight size={23} aria-hidden="true" /></button>
            </div>
          </section>
        )
      )}
    </>
  );
}
