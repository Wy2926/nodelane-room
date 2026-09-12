import { Account } from "./Account";
import { t, getLanguage, setLanguage, translate, isLanguage } from "../../i18n";
import { useEffect, useState } from "react";
import { isTauri } from "@tauri-apps/api/core";
import { enable, disable, isEnabled } from "@tauri-apps/plugin-autostart";
import {
  Check,
  Copy,
  Desktop,
  DownloadSimple,
  Sun,
  Power,
  SlidersHorizontal,
  UserCircle,
} from "@phosphor-icons/react";
import { Updates } from "./Updates";
import type { Status } from "../../shared/model";
import type { Actions } from "../../app/use-actions";

const categories = [
  { id: "account", title: "account.title", icon: UserCircle },
  {
    id: "preferences",
    title: "settings.desktopPreferences",
    icon: SlidersHorizontal,
  },
  { id: "device", title: "settings.deviceInformation", icon: Desktop },
  { id: "updates", title: "settings.versionAndUpdates", icon: DownloadSimple },
] as const;

export function Settings({
  status,
  actions,
  category,
  setCategory,
  serviceUnavailable,
}: {
  status?: Status;
  actions: Actions;
  category: (typeof categories)[number]["id"];
  setCategory: (category: (typeof categories)[number]["id"]) => void;
  serviceUnavailable: boolean;
}) {
  const { busy, setBusy, setError, copy } = actions;
  const [autoStart, setAutoStart] = useState<boolean>();
  useEffect(() => {
    let active = true;
    if (isTauri())
      void isEnabled()
        .then((value) => {
          if (active) setAutoStart(value);
        })
        .catch(() => {});
    return () => {
      active = false;
    };
  }, []);
  return (
    <div className="settings">
      <div className="page-intro">
        <h2>{t("navigation.settings")}</h2>
        <p>{t("settings.intro")}</p>
      </div>
      <div className="settings-layout">
        <nav
          className="settings-nav"
          aria-label={t("settings.settingsCategories")}
        >
          {categories.map(({ id, title, icon: Icon }) => (
            <button
              key={id}
              aria-current={category === id ? "page" : undefined}
              onClick={() => setCategory(id)}
            >
              <Icon size={21} aria-hidden="true" />
              {t(title)}
            </button>
          ))}
        </nav>
        <div className="settings-content">
          {category === "account" && (
            <Account
              status={status}
              actions={actions}
              unavailable={serviceUnavailable}
            />
          )}
          {category === "preferences" && (
            <section aria-labelledby="preferences-title">
              <div className="settings-section-head">
                <h3 id="preferences-title">
                  {t("settings.desktopPreferences")}
                </h3>
                <p>{t("settings.aFamiliarSpaceReadyWhenYouAre")}</p>
              </div>
              <label className="setting-row language-setting">
                <span className="setting-row-copy">
                  <strong>{t("language.setting")}</strong>
                  <small>{t("language.changeLater")}</small>
                </span>
                <select
                  value={getLanguage()}
                  disabled={!!busy}
                  onChange={(event) => {
                    if (isLanguage(event.target.value))
                      setLanguage(event.target.value);
                  }}
                >
                  {(["zh-CN", "en-US"] as const).map((language) => (
                    <option key={language} value={language} lang={language}>
                      {translate(language, "language.name")}
                    </option>
                  ))}
                </select>
              </label>
              <div className="theme-preview">
                <div className="theme-preview-copy">
                  <Sun size={28} weight="light" aria-hidden="true" />
                  <h4>{t("desk.themeName")}</h4>
                  <p>{t("desk.themeDescription")}</p>
                </div>
                <span className="theme-current">
                  <Check size={16} aria-hidden="true" />
                  {t("settings.currentTheme")}
                </span>
              </div>
              <label className="switch-row">
                <Power size={24} weight="light" aria-hidden="true" />
                <span className="setting-row-copy">
                  <strong>{t("settings.launchAtSignIn")}</strong>
                  <small>{t("settings.openTheClientWhenYouSignIn")}</small>
                  {autoStart === undefined && (
                    <small>
                      {t("settings.launchAtSignInStatusIsUnavailable")}
                    </small>
                  )}
                </span>
                <input
                  type="checkbox"
                  role="switch"
                  aria-label={t("settings.openTheClientWhenYouSignIn")}
                  checked={autoStart ?? false}
                  disabled={autoStart === undefined || !!busy}
                  onChange={async (e) => {
                    const checked = e.target.checked;
                    setBusy(t("settings.savingLaunchPreferences"));
                    try {
                      await (checked ? enable() : disable());
                      setAutoStart(await isEnabled());
                    } catch {
                      setError({
                        code: "local_preference_save_failed",
                        error: t("settings.autostartError"),
                      });
                    } finally {
                      setBusy("");
                    }
                  }}
                />
              </label>
              <div className="setting-row">
                <Desktop size={24} weight="light" aria-hidden="true" />
                <div className="setting-row-copy">
                  <strong>{t("settings.closeBehavior")}</strong>
                  <small>{t("settings.closeBehaviorHelp")}</small>
                </div>
                <span className="setting-value">
                  {t("settings.defaultBehavior")}
                </span>
              </div>
            </section>
          )}
          {category === "device" && (
            <section aria-labelledby="device-title">
              <div className="settings-section-head">
                <h3 id="device-title">{t("settings.deviceInformation")}</h3>
                <p>{t("settings.deviceHelp")}</p>
              </div>
              <dl className="device-details">
                <div>
                  <dt>{t("settings.deviceNickname")}</dt>
                  <dd>{status?.name || t("settings.notConfigured")}</dd>
                </div>
                <div>
                  <dt>{t("settings.gamingService")}</dt>
                  <dd className="selectable">
                    {status?.server || t("settings.notConfigured")}
                  </dd>
                </div>
                <div>
                  <dt>{t("settings.deviceId")}</dt>
                  <dd className="device-identity">
                    <span className="mono selectable">
                      {status?.device_id || t("settings.notConfigured")}
                    </span>
                    <button
                      className="icon-button"
                      disabled={!status?.device_id}
                      aria-label={t("settings.copyDeviceId")}
                      title={t("settings.copyDeviceId")}
                      onClick={() => void copy(status!.device_id)}
                    >
                      <Copy size={19} aria-hidden="true" />
                    </button>
                  </dd>
                </div>
              </dl>
            </section>
          )}
          {category === "updates" && <Updates />}
        </div>
      </div>
    </div>
  );
}
