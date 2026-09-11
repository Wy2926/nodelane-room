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
      <a className="skip-link" href="#main-content">跳到主要内容</a>
      <header className="console-bar" data-tauri-drag-region>
        <div className="brand" aria-label="NodeLane Room" data-tauri-drag-region>
          <img src={logo} alt="" /><span>NodeLane<small>ROOM</small></span>
        </div>
        <nav aria-label="主导航">
          {navigation.map(({ id, icon: Icon }) => (
            <button key={id} aria-current={page === id ? "page" : undefined} onClick={() => setPage(id)}>
              <Icon size={21} aria-hidden="true" /><span>{titles[id]}</span>
            </button>
          ))}
        </nav>
        <div className="titlebar-space" data-tauri-drag-region />
        <button className="identity" aria-label="个人资料" title="个人资料与桌面偏好" onClick={() => setPage("settings")}>
          <PlayerAvatar name={status?.name || "N"} identity={status?.device_id} size="small" />
          <span className="identity-copy"><strong>{status?.name || "本机玩家"}</strong><small>{error ? "服务不可用" : status?.selected_room ? "已加入房间" : "尚未加入房间"}</small></span>
        </button>
        <WindowControls />
      </header>
      <main id="main-content" tabIndex={-1}>
        <h1 className="sr-only">{titles[page]}</h1>
        {children}
      </main>
    </div>
  );
}

function WindowControls() {
  const [maximized, setMaximized] = useState(false);
  const [error, setError] = useState("");
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
    setError("");
    try { await getCurrentWindow()[action](); }
    catch { setError("窗口操作未完成，请重试。"); }
  };
  return <div className="window-controls" aria-label="窗口控制">
    <button aria-label="最小化" title="最小化" disabled={!native} onClick={() => void run("minimize")}><Minus size={17} /></button>
    <button aria-label={maximized ? "还原窗口" : "最大化"} title={maximized ? "还原窗口" : "最大化"} disabled={!native} onClick={() => void run("toggleMaximize")}>{maximized ? <CopySimple size={17} /> : <Square size={16} />}</button>
    <button className="window-close" aria-label="关闭窗口" title="关闭窗口（继续联机）" disabled={!native} onClick={() => void run("close")}><X size={19} /></button>
    {error && <span className="window-error" role="alert">{error}</span>}
  </div>;
}
