import { t, getLanguage, setLanguage, translate, isLanguage } from "../../i18n";
import { useEffect, useState } from "react";
import { isTauri } from "@tauri-apps/api/core";
import { enable, disable, isEnabled } from "@tauri-apps/plugin-autostart";
import { ArrowClockwise, Check, Copy, Desktop, DownloadSimple, Info, Moon, Power, SlidersHorizontal } from "@phosphor-icons/react";
import type { Status } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import { PlayerAvatar } from "../../shared/ui/PlayerAvatar";
import { clientVersion } from "../../native/api";

const categories = [
  { id: "preferences", title: "settings.desktopPreferences", icon: SlidersHorizontal },
  { id: "device", title: "settings.deviceInformation", icon: Desktop },
  { id: "updates", title: "settings.versionAndUpdates", icon: DownloadSimple },
  { id: "session", title: "settings.exitAndConnection", icon: Power },
] as const;

export function Settings({ status, actions, usable }: {
  status?: Status; actions: Actions; usable: boolean;
}) {
  const { busy, setBusy, setError, confirm, quit, copy } = actions;
  const [category, setCategory] = useState<typeof categories[number]["id"]>("preferences");
  const [autoStart, setAutoStart] = useState<boolean>();
  const [updateRequested, setUpdateRequested] = useState(false);
  useEffect(() => {
    let active = true;
    if (isTauri()) void isEnabled().then((value) => { if (active) setAutoStart(value); }).catch(() => {});
    return () => { active = false; };
  }, []);
  return (
    <div className="settings">
      <div className="page-intro"><span className="eyebrow">{t("settings.makeYourselfAtHome")}</span><h2>{t("navigation.settings")}</h2><p>{t("settings.intro")}</p></div>
      <div className="settings-layout">
        <aside className="settings-sidebar">
          <div className="settings-profile">
            <PlayerAvatar name={status?.name || "N"} identity={status?.device_id} size="large" />
            <h3>{status?.name || t("shell.localPlayer")}</h3><p>{status?.device_id ? t("settings.yourGamingIdentityOnThisDevice") : t("settings.deviceIdentityHasNotBeenLoaded")}</p>
          </div>
          <nav className="settings-nav" aria-label={t("settings.settingsCategories")}>
            {categories.map(({ id, title, icon: Icon }) => <button key={id} aria-current={category === id ? "page" : undefined} onClick={() => setCategory(id)}><Icon size={21} aria-hidden="true" />{t(title)}</button>)}
          </nav>
          <span className="settings-signature">NodeLane Room <span className="mono">{clientVersion}</span></span>
        </aside>
        <div className="settings-content console-surface">
          {category === "preferences" && <section aria-labelledby="preferences-title">
            <div className="settings-section-head"><h3 id="preferences-title">{t("settings.desktopPreferences")}</h3><p>{t("settings.aFamiliarSpaceReadyWhenYouAre")}</p></div>
            <label className="setting-row language-setting">
              <span className="setting-row-copy"><strong>{t("language.setting")}</strong><small>{t("language.changeLater")}</small></span>
              <select value={getLanguage()} disabled={!!busy} onChange={(event) => { if (isLanguage(event.target.value)) setLanguage(event.target.value); }}>
                {(["zh-CN", "en-US"] as const).map((language) => <option key={language} value={language} lang={language}>{translate(language, "language.name")}</option>)}
              </select>
            </label>
            <div className="theme-preview">
              <div className="theme-preview-copy"><Moon size={28} weight="light" aria-hidden="true" /><h4>{t("settings.midnight")}</h4><p>{t("settings.themeDescription")}</p></div>
              <span className="theme-current"><Check size={16} aria-hidden="true" />{t("settings.currentTheme")}</span>
            </div>
            <label className="switch-row">
              <Power size={24} weight="light" aria-hidden="true" />
              <span className="setting-row-copy"><strong>{t("settings.launchAtSignIn")}</strong><small>{t("settings.openTheClientWhenYouSignIn")}</small>{autoStart === undefined && <small>{t("settings.launchAtSignInStatusIsUnavailable")}</small>}</span>
              <input type="checkbox" role="switch" aria-label={t("settings.openTheClientWhenYouSignIn")} checked={autoStart ?? false} disabled={autoStart === undefined || !!busy}
                onChange={async (e) => {
                  const checked = e.target.checked;
                  setBusy(t("settings.savingLaunchPreferences"));
                  try { await (checked ? enable() : disable()); setAutoStart(await isEnabled()); }
                  catch { setError({ code: "autostart", error: t("settings.autostartError") }); }
                  finally { setBusy(""); }
                }} />
            </label>
            <div className="setting-row"><Desktop size={24} weight="light" aria-hidden="true" /><div className="setting-row-copy"><strong>{t("settings.closeBehavior")}</strong><small>{t("settings.closeBehaviorHelp")}</small></div><span className="setting-value">{t("settings.defaultBehavior")}</span></div>
          </section>}
          {category === "device" && <section aria-labelledby="device-title">
            <div className="settings-section-head"><h3 id="device-title">{t("settings.deviceInformation")}</h3><p>{t("settings.deviceHelp")}</p></div>
            <dl className="device-details">
              <div><dt>{t("settings.deviceNickname")}</dt><dd>{status?.name || t("settings.notConfigured")}</dd></div>
              <div><dt>{t("settings.gamingService")}</dt><dd className="selectable">{status?.server || t("settings.notConfigured")}</dd></div>
              <div><dt>{t("settings.deviceId")}</dt><dd className="device-identity"><span className="mono selectable">{status?.device_id || t("settings.notConfigured")}</span><button className="icon-button" disabled={!status?.device_id} aria-label={t("settings.copyDeviceId")} title={t("settings.copyDeviceId")} onClick={() => void copy(status!.device_id)}><Copy size={19} aria-hidden="true" /></button></dd></div>
              <div><dt>{t("settings.serviceVersion")}</dt><dd className="mono">{status?.version || t("settings.unavailable")}</dd></div>
            </dl>
          </section>}
          {category === "updates" && <section aria-labelledby="updates-title">
            <div className="settings-section-head"><h3 id="updates-title">{t("settings.versionAndUpdates")}</h3><p>{t("settings.updatesHelp")}</p></div>
            <div className="update-version"><span className="update-icon"><DownloadSimple size={36} weight="light" aria-hidden="true" /></span><div><span className="muted">NodeLane Room</span><h4>{t("settings.currentVersion")}<span className="mono">{clientVersion}</span></h4><p>{t("settings.serviceVersion")}<span className="mono">{status?.version || t("settings.unavailable")}</span></p></div></div>
            <div className="update-action"><div><strong>{t("settings.checkForClientUpdates")}</strong><p>{t("settings.updatesUnavailable")}</p></div><button onClick={() => setUpdateRequested(true)}><ArrowClockwise size={19} aria-hidden="true" />{t("settings.checkForUpdates")}</button></div>
            {updateRequested && <div className="update-message" role="status"><Info size={22} aria-hidden="true" /><p>{t("settings.updatesUnavailableHelp")}</p></div>}
            <div className="settings-help"><h4>{t("settings.howToUpdate")}</h4><p>{t("settings.manualUpdateHelp")}</p></div>
          </section>}
          {category === "session" && <section aria-labelledby="session-title">
            <div className="settings-section-head"><h3 id="session-title">{t("settings.exitAndConnection")}</h3><p>{t("settings.chooseHowYouWantToSayGoodbyeFor")}</p></div>
            <div className="session-option"><span className="session-icon"><Desktop size={28} weight="light" aria-hidden="true" /></span><div><h4>{t("settings.keepTheGameGoing")}</h4><p>{t("settings.stayConnectedHelp")}</p><button onClick={() => void quit()} disabled={!!busy}>{t("settings.exitAppStayConnected")}</button></div></div>
            <div className="session-option"><span className="session-icon"><Power size={28} weight="light" aria-hidden="true" /></span><div><h4>{t("settings.callItADay")}</h4><p>{t("settings.leaveAndExitHelp")}</p><button className="danger subtle" disabled={!usable} onClick={() => status?.selected_room ? confirm(t("useActions.leaveRoomAndExit"), t("settings.disconnectThisDeviceAndExitTheApp"), { action: "leave" }, true) : void quit()}>{t("useActions.leaveRoomAndExit")}</button></div></div>
          </section>}
        </div>
      </div>
    </div>
  );
}
