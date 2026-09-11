import { t, getLanguage, translate, type Language, type MessageKey } from "../i18n";
import { useEffect, useState, type ReactNode } from "react";
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { GameController, House, GearSix, Pulse, Minus, Square, CopySimple, X } from "@phosphor-icons/react";
import { PlayerAvatar } from "../shared/ui/PlayerAvatar";
import type { useService } from "../native/use-service";
import { titles, type Page } from "./navigation";
import logo from "../../icon.svg";

const navigation = [
  { id: "rooms", icon: House }, { id: "games", icon: GameController },
  { id: "doctor", icon: Pulse }, { id: "settings", icon: GearSix },
] as const;

export function Shell({ page, setPage, service, children }: {
  page: Page; setPage: (page: Page) => void;
  service: ReturnType<typeof useService>; children: ReactNode;
}) {
  const { status, error } = service;
  useEffect(() => { window.scrollTo({ top: 0, behavior: "instant" }); }, [page]);
  return (
    <div className="shell" data-page={page}>
      <a className="skip-link" href="#main-content">{t("shell.skipToMainContent")}</a>
      <header className="console-bar" data-tauri-drag-region>
        <div className="brand" aria-label="NodeLane Room" data-tauri-drag-region>
          <img src={logo} alt="" /><span>NodeLane<small>ROOM</small></span>
        </div>
        <nav aria-label={t("shell.mainNavigation")}>
          {navigation.map(({ id, icon: Icon }) => (
            <button key={id} aria-current={page === id ? "page" : undefined} onClick={() => setPage(id)}>
              <Icon size={21} aria-hidden="true" /><span>{t(titles[id])}</span>
            </button>
          ))}
        </nav>
        <div className="titlebar-space" data-tauri-drag-region />
        <button className="identity" aria-label={t("shell.profile")} title={t("shell.profileAndDesktopPreferences")} onClick={() => setPage("settings")}>
          <PlayerAvatar name={status?.name || "N"} identity={status?.device_id} size="small" />
          <span className="identity-copy"><strong>{status?.name || t("shell.localPlayer")}</strong><small>{error ? t("shell.serviceUnavailable") : status?.selected_room ? t("shell.joinedARoom") : t("shell.noRoomJoined")}</small></span>
        </button>
        <WindowControls />
      </header>
      <main id="main-content" tabIndex={-1}>
        <h1 className="sr-only">{t(titles[page])}</h1>
        {children}
      </main>
    </div>
  );
}

export function WindowControls({ language = getLanguage() }: { language?: Language }) {
  const text = (key: MessageKey) => translate(language, key);
  const [maximized, setMaximized] = useState(false);
  const [error, setError] = useState(false);
  const native = isTauri();
  useEffect(() => {
    if (!native) return;
    let active = true;
    const window = getCurrentWindow();
    const sync = async () => {
      try { const value = await window.isMaximized(); if (active) setMaximized(value); } catch { /* Controls remain usable if state cannot be read. */ }
    };
    void sync();
    const listener = window.onResized(() => void sync()).catch(() => () => {});
    return () => { active = false; void listener.then((unlisten) => unlisten()); };
  }, [native]);
  const run = async (action: "minimize" | "toggleMaximize" | "close") => {
    setError(false);
    try { await getCurrentWindow()[action](); }
    catch { setError(true); }
  };
  return <div className="window-controls" aria-label={text("shell.windowControls")}>
    <button aria-label={text("shell.minimize")} title={text("shell.minimize")} disabled={!native} onClick={() => void run("minimize")}><Minus size={17} /></button>
    <button aria-label={maximized ? text("shell.restoreWindow") : text("shell.maximize")} title={maximized ? text("shell.restoreWindow") : text("shell.maximize")} disabled={!native} onClick={() => void run("toggleMaximize")}>{maximized ? <CopySimple size={17} /> : <Square size={16} />}</button>
    <button className="window-close" aria-label={text("shell.closeWindow")} title={text("shell.closeWindowStayConnected")} disabled={!native} onClick={() => void run("close")}><X size={19} /></button>
    {error && <span className="window-error" role="alert">{text("shell.windowActionFailedPleaseTryAgain")}</span>}
  </div>;
}
