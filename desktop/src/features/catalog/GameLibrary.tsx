import { useState } from "react";
import { ArrowRight, CaretLeft, CaretRight, MagnifyingGlass, Plus, Ticket } from "@phosphor-icons/react";
import type { Status } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import type { Catalog } from "./use-catalog";
import { Art } from "./Artwork";
import { Empty } from "../../shared/ui/Empty";
import { PortList } from "../../shared/ui/PortList";

export function GameLibrary({ catalog, status, actions, usable, refreshAll }: {
  catalog: Catalog; status: Status; actions: Actions; usable: boolean; refreshAll: () => void;
}) {
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState("");
  const { games, gamesError, loading } = catalog;
  const filtered = games.filter((g) => g.enabled && (g.name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()) || g.id.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())));
  const game = filtered.find((g) => g.id === selected) || filtered[0];
  const disabled = !usable || !!status.selected_room || !!gamesError;
  const selectAt = (index: number, focus = false) => {
    const next = filtered[(index + filtered.length) % filtered.length];
    if (!next) return;
    setSelected(next.id);
    const tile = document.getElementById(`game-${next.id}`);
    if (focus) tile?.focus({ preventScroll: true });
    tile?.scrollIntoView?.({ block: "nearest", inline: "nearest", behavior: "smooth" });
  };
  return (
    <div className="library">
      {game && <div className="scene-art" aria-hidden="true"><Art key={game.id} game={game} kind="background" /></div>}
      <div className="section-toolbar library-toolbar">
        <span className="eyebrow">CHOOSE YOUR NEXT WORLD <span className="catalog-count">{filtered.length} 款游戏</span></span>
        <label className="search"><MagnifyingGlass size={18} aria-hidden="true" /><span className="sr-only">搜索游戏</span><input type="search" placeholder="搜索游戏" value={query} onChange={(e) => setQuery(e.target.value)} /></label>
      </div>
      {gamesError && <div className="banner warning" role="alert">游戏库暂未更新，旧资料仅供查看。{gamesError.error}<button onClick={refreshAll}>重试</button></div>}
      {status.selected_room && <div className="banner">每台设备同时连接一个房间。创建其他房间前，请先离开当前房间。</div>}
      {game ? <>
        <div className="shelf-row">
          <div className="game-shelf" aria-label="选择游戏">
            {filtered.map((g, index) => <button className="game-tile" id={`game-${g.id}`} key={g.id} aria-label={g.name} aria-pressed={g.id === game.id} tabIndex={g.id === game.id ? 0 : -1}
              onClick={() => setSelected(g.id)} onKeyDown={(e) => {
                const next = e.key === "ArrowRight" ? index + 1 : e.key === "ArrowLeft" ? index - 1 : e.key === "Home" ? 0 : e.key === "End" ? filtered.length - 1 : undefined;
                if (next !== undefined) { e.preventDefault(); selectAt(next, true); }
              }}>
              <Art game={g} /><strong>{g.name}</strong>
            </button>)}
          </div>
          {filtered.length > 1 && <div className="shelf-controls"><button className="icon-button" aria-label="上一款游戏" onClick={() => selectAt(filtered.indexOf(game) - 1)}><CaretLeft size={18} /></button><button className="icon-button" aria-label="下一款游戏" onClick={() => selectAt(filtered.indexOf(game) + 1)}><CaretRight size={18} /></button></div>}
        </div>
        <section className="library-hero" aria-label="所选游戏">
          <div className="library-hero-content" key={game.id}>
            <div className="game-category"><span className="tag">{game.id === "custom" ? "自由联机" : "多人联机"}</span><span>NODELANE ROOM</span></div>
            <h2>{game.name}</h2>
            <p>{game.summary || (game.id === "custom" ? "你喜欢的游戏，你们自己的世界。配置游戏端口，邀请朋友一起加入。" : "开启一个属于你们的房间，与朋友一起探索下一段旅程。")}</p>
            <div className="actions">
              <button className="primary" disabled={disabled} onClick={() => { actions.setError(undefined); actions.setDialog({ type: "create", game }); }}><Plus size={19} aria-hidden="true" />创建房间</button>
              <button className="subtle" disabled={!usable || !!status.selected_room} onClick={() => { actions.setError(undefined); actions.setDialog({ type: "join" }); }}><Ticket size={20} aria-hidden="true" />邀请码入房</button>
            </div>
          </div>
          <aside className="library-note"><span className="eyebrow">BETTER TOGETHER</span><p>下一段冒险，<br />一起出发。</p><ArrowRight size={26} weight="light" aria-hidden="true" /></aside>
        </section>
        <details className="game-details"><summary>游戏介绍与连接端口</summary><p className="muted">{game.summary || "通过房间内的虚拟 IP 连接游戏。"}</p><PortList ports={game.ports} /><p className="hint">游戏主机的实际监听设置须与配置一致。游戏收录不代表兼容性已验收。</p></details>
      </> : <Empty title={loading ? "正在读取游戏库" : gamesError ? "游戏库暂不可用" : "没有匹配的游戏"}><p>{gamesError ? "连接恢复后重试。" : "试试其他名称，或清空搜索。"}</p></Empty>}
    </div>
  );
}
