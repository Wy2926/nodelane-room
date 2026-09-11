import { failure } from "../../native/api";
import { t } from "../../i18n";
import { useState } from "react";
import { CaretLeft, CaretRight, MagnifyingGlass, Plus } from "@phosphor-icons/react";
import type { Status } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import type { Catalog } from "./use-catalog";
import { Art } from "./Artwork";
import { Empty } from "../../shared/ui/Empty";

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
        <div><span className="eyebrow">{t("gameLibrary.discoverYourNextAdventure")}</span><h2>{t("navigation.gameLibrary")}<span className="catalog-count">{t("gameLibrary.gameCount", { count: filtered.length })}</span></h2></div>
        <label className="search"><MagnifyingGlass size={18} aria-hidden="true" /><span className="sr-only">{t("gameLibrary.searchGames")}</span><input type="search" placeholder={t("gameLibrary.searchGames")} value={query} onChange={(e) => setQuery(e.target.value)} /></label>
      </div>
      {gamesError && <div className="banner warning" role="alert">{t("gameLibrary.stale")}{failure(gamesError).error}<button onClick={refreshAll}>{t("gameLibrary.retry")}</button></div>}
      {status.selected_room && <div className="banner">{t("gameLibrary.oneRoomPerDevice")}</div>}
      {game ? <>
        <div className="shelf-row">
          <div className="game-shelf" aria-label={t("gameLibrary.chooseAGame")}>
            {filtered.map((g, index) => <button className="game-tile" id={`game-${g.id}`} key={g.id} aria-label={g.name} aria-pressed={g.id === game.id} tabIndex={g.id === game.id ? 0 : -1}
              onClick={() => setSelected(g.id)} onKeyDown={(e) => {
                const next = e.key === "ArrowRight" ? index + 1 : e.key === "ArrowLeft" ? index - 1 : e.key === "Home" ? 0 : e.key === "End" ? filtered.length - 1 : undefined;
                if (next !== undefined) { e.preventDefault(); selectAt(next, true); }
              }}>
              <Art game={g} /><strong>{g.name}</strong>
            </button>)}
          </div>
          {filtered.length > 1 && <div className="shelf-controls"><button className="icon-button" aria-label={t("gameLibrary.previousGame")} onClick={() => selectAt(filtered.indexOf(game) - 1)}><CaretLeft size={18} /></button><button className="icon-button" aria-label={t("gameLibrary.nextGame")} onClick={() => selectAt(filtered.indexOf(game) + 1)}><CaretRight size={18} /></button></div>}
        </div>
        <section className="library-hero" aria-label={t("gameLibrary.selectedGame")}>
          <div className="library-hero-content" key={game.id}>
            <div className="game-category"><span className="tag">{game.id === "custom" ? t("gameLibrary.playYourWay") : t("gameLibrary.multiplayer")}</span><span>{t("gameLibrary.yourSelectedGame")}</span></div>
            <h2>{game.name}</h2>
            <p>{t("gameLibrary.createRoomHelp")}</p>
            <div className="actions">
              <button className="primary" disabled={disabled} onClick={() => { actions.setError(undefined); actions.setDialog({ type: "create", game }); }}><Plus size={19} aria-hidden="true" />{t("gameLibrary.createRoom")}</button>
            </div>
          </div>
        </section>
      </> : <Empty title={loading ? t("gameLibrary.loadingGameLibrary") : gamesError ? t("gameLibrary.gameLibraryUnavailable") : t("gameLibrary.noMatchingGames")}><p>{gamesError ? t("gameLibrary.retryWhenConnected") : t("gameLibrary.tryAnotherNameOrClearTheSearch")}</p></Empty>}
    </div>
  );
}
