import { useEffect, useState, type ReactNode } from "react";
import { CaretRight, GameController, House, GearSix, Pulse } from "@phosphor-icons/react";
import { PlayerAvatar } from "../shared/ui/PlayerAvatar";
import type { useService } from "../native/use-service";
import { titles, type Page } from "./navigation";
import logo from "../../icon.svg";

export function Shell({ page, setPage, service, children }: {
  page: Page; setPage: (page: Page) => void;
  service: ReturnType<typeof useService>; children: ReactNode;
}) {
  const { status, error, updatedAt } = service;
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 30000);
    return () => clearInterval(timer);
  }, []);
  const network = error ? "服务不可用" : !status ? "正在连接服务" : status.control === "unreachable" ? "控制端失联" : status.engine === "running" ? "游戏网络运行中" : status.selected_room ? "已加入房间 · 网络未连接" : "尚未连接房间";
  return (
    <div className="shell" data-page={page}>
      <a className="skip-link" href="#main-content">跳到主要内容</a>
      <header className="console-bar">
        <div className="brand" aria-label="NodeLane Room">
          <img src={logo} alt="" /><span>NodeLane<small>ROOM</small></span>
        </div>
        <nav aria-label="主导航">
          {(["rooms", "games"] as const).map((id) => (
            <button key={id} aria-current={page === id ? "page" : undefined} onClick={() => setPage(id)}>{titles[id]}</button>
          ))}
        </nav>
        <button className="identity" aria-label="个人资料" title="个人资料与桌面偏好" disabled={!status?.device_id} aria-current={page === "settings" ? "page" : undefined} onClick={() => setPage("settings")}>
          <PlayerAvatar name={status?.name || "N"} identity={status?.device_id} size="small" />
          <span className="identity-copy"><strong>{status?.name || "欢迎回来"}</strong><small>本机玩家</small></span>
          <CaretRight size={13} weight="light" aria-hidden="true" />
        </button>
        <time className="console-clock" dateTime={now.toISOString()}>{now.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false })}</time>
      </header>
      <main id="main-content" tabIndex={-1}>
        <div className={page === "games" || page === "rooms" ? "sr-only" : "page-heading"}><span className="eyebrow">CONTROL CENTER</span><h1>{titles[page]}</h1></div>
        {children}
      </main>
      <footer className="control-bar">
        <div className="network-status" title={updatedAt ? `状态更新于 ${new Date(updatedAt).toLocaleTimeString("zh-CN")}` : "等待本机状态"}>
          <i className="status-dot" data-ok={!error && status?.engine === "running" && status?.control !== "unreachable"} />
          <span>{network}<small>{error ? "显示的数据可能已陈旧" : status?.ip || "NodeLane Room"}</small></span>
        </div>
        <nav className="control-dock" aria-label="控制中心">
          <button className="icon-button" aria-label="返回房间" title="我的房间" aria-current={page === "rooms" ? "page" : undefined} onClick={() => setPage("rooms")}><House size={23} weight="light" /></button>
          <GameController className="dock-mark" size={26} weight="light" aria-hidden="true" />
          <button className="icon-button" aria-label="网络诊断" title="网络诊断" aria-current={page === "doctor" ? "page" : undefined} onClick={() => setPage("doctor")}><Pulse size={25} weight="light" /></button>
          <button className="icon-button" aria-label="设置" title="设置" aria-current={page === "settings" ? "page" : undefined} onClick={() => setPage("settings")}><GearSix size={24} weight="light" /></button>
        </nav>
        <div className="keyboard-hints"><span><kbd>Tab</kbd> 切换</span><span><kbd>Enter</kbd> 选择</span></div>
      </footer>
    </div>
  );
}
