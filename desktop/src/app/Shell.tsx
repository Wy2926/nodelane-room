import {
  t,
  getLanguage,
  translate,
  type Language,
  type MessageKey,
} from "../i18n";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import {
  GearSix,
  Broadcast,
  X,
  CaretDown,
  Circle,
  Users,
} from "@phosphor-icons/react";
import { PlayerAvatar } from "../shared/ui/PlayerAvatar";
import brandMark from "../assets/brand-mark.png";
import { clientVersion } from "../native/api";
import type { useService } from "../native/use-service";
import { titles, type Page } from "./navigation";
import type { Actions } from "./use-actions";

export function Shell({
  page,
  setPage,
  openProfile,
  service,
  actions,
  onboarding,
  children,
}: {
  page: Page;
  setPage: (page: Page) => void;
  openProfile: () => void;
  service: ReturnType<typeof useService>;
  actions: Actions;
  onboarding: boolean;
  children: ReactNode;
}) {
  const { status, error, stale } = service;
  const online = !error && !stale && status?.control === "online";
  const [profileOpen, setProfileOpen] = useState(false);
  const profile = useRef<HTMLDetailsElement>(null);
  const content = useRef<HTMLElement>(null);
  const closeProfile = () => {
    if (profile.current) profile.current.open = false;
  };
  useEffect(() => {
    const dismiss = (event: PointerEvent) => {
      if (!profile.current?.contains(event.target as Node)) closeProfile();
    };
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, []);
  useEffect(() => {
    if (content.current) content.current.scrollTop = 0;
  }, [page]);
  return (
    <div
      className={`shell${onboarding ? " shell-onboarding" : ""}`}
      data-page={page}
    >
      <a className="skip-link" href="#main-content">
        {t("shell.skipToMainContent")}
      </a>
      <header className="app-header" data-tauri-drag-region>
        <button
          className="brand"
          aria-label={t("navigation.myRooms")}
          onClick={() => setPage("rooms")}
        >
          <img src={brandMark} alt="" />
          <span>
            NodeLane <b>Room</b>
          </span>
          <small className="brand-version">v{clientVersion}</small>
        </button>
        <div className="titlebar-space" data-tauri-drag-region />
        <nav aria-label={t("shell.mainNavigation")}>
          <button
            title={t(titles.doctor)}
            aria-current={page === "doctor" ? "page" : undefined}
            onClick={() => setPage("doctor")}
          >
            <Broadcast size={23} aria-hidden="true" />
            {t(titles.doctor)}
          </button>
          <button
            title={t(titles.settings)}
            aria-current={page === "settings" ? "page" : undefined}
            onClick={() => setPage("settings")}
          >
            <GearSix size={23} aria-hidden="true" />
            {t(titles.settings)}
          </button>
        </nav>
        {!onboarding && (
          <details
            className="profile-menu"
            ref={profile}
            onToggle={(e) => setProfileOpen(e.currentTarget.open)}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                closeProfile();
                profile.current?.querySelector("summary")?.focus();
              }
            }}
            onBlur={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget)) closeProfile();
            }}
          >
            <summary className="identity" aria-label={t("shell.profile")}>
              <PlayerAvatar name={status?.name || "N"} size="small" />
              <span>{status?.name || t("shell.localPlayer")}</span>
              <CaretDown size={14} aria-hidden="true" />
            </summary>
            <div className="profile-options" hidden={!profileOpen}>
              <button
                onClick={() => {
                  closeProfile();
                  openProfile();
                }}
              >
                {t("account.title")}
              </button>
              <p>{t("settings.exitAndConnection")}</p>
              <button
                disabled={!!actions.busy}
                onClick={() => {
                  closeProfile();
                  void actions.quit();
                }}
              >
                {t("settings.exitAppStayConnected")}
                <small>{t("desk.exitKeepsNetwork")}</small>
              </button>
              <button
                className="danger"
                disabled={
                  !status ||
                  !!error ||
                  stale ||
                  !!actions.busy ||
                  !!actions.pending
                }
                onClick={() => {
                  closeProfile();
                  if (status?.selected_room)
                    actions.confirm(
                      t("useActions.leaveRoomAndExit"),
                      t("settings.disconnectThisDeviceAndExitTheApp"),
                      { action: "leave" },
                      true,
                    );
                  else void actions.quit();
                }}
              >
                {t("useActions.leaveRoomAndExit")}
                <small>{t("desk.exitLeavesRoom")}</small>
              </button>
            </div>
          </details>
        )}
        {isTauri() && <WindowControls />}
      </header>
      <main
        id="main-content"
        ref={content}
        tabIndex={0}
        aria-labelledby="page-title"
      >
        <h1 id="page-title" className="sr-only">
          {t(titles[page])}
        </h1>
        {children}
      </main>
      {!onboarding && (
        <footer className="app-status" aria-label={t("desk.connectionStatus")}>
          <span>
            <Circle
              size={11}
              weight="fill"
              data-online={online}
              aria-hidden="true"
            />
            {t(
              error
                ? "shell.serviceUnavailable"
                : stale
                  ? "experience.stale"
                  : online
                    ? "desk.serviceOnline"
                    : "experience.reconnecting",
            )}
          </span>
          <span>
            <Users size={18} aria-hidden="true" />
            {status?.selected_room
              ? status.room?.name || t("roomPage.syncingRoom")
              : t("shell.noRoomJoined")}
          </span>
        </footer>
      )}
    </div>
  );
}
export function WindowControls({
  language = getLanguage(),
}: {
  language?: Language;
}) {
  const text = (key: MessageKey) => translate(language, key);
  const [error, setError] = useState(false);
  const native = isTauri();
  const close = async () => {
    setError(false);
    try {
      await getCurrentWindow().close();
    } catch {
      setError(true);
    }
  };
  return (
    <div className="window-controls" aria-label={text("shell.windowControls")}>
      <button
        className="window-close"
        aria-label={text("shell.closeWindow")}
        title={text("shell.closeWindowStayConnected")}
        disabled={!native}
        onClick={() => void close()}
      >
        <X size={19} />
      </button>
      {error && (
        <span className="window-error" role="alert">
          {text("shell.windowActionFailedPleaseTryAgain")}
        </span>
      )}
    </div>
  );
}
