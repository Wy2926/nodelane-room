import { Account } from "./Account";
import { t } from "../../i18n";
import type { Actions } from "../../app/use-actions";
export function Setup({ actions }: { actions: Actions }) {
  const { perform, busy } = actions;
  return (
    <section className="onboarding">
      <h2>{t("setup.welcomeToNodelane")}</h2>
      <p className="muted">{t("setup.nicknameHelp")}</p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          const data = new FormData(e.currentTarget);
          void perform(t("setup.initializingDevice"), {
            action: "init",
            server: String(data.get("server")).trim(),
            name: String(data.get("name")).trim(),
          });
        }}
      >
        <label>
          {t("settings.deviceNickname")}
          <input
            name="name"
            required
            maxLength={40}
            placeholder={t("setup.aNameYourFriendsWillRecognize")}
          />
        </label>
        <details className="server-choice">
          <summary>{t("setup.gamingServiceRoomNodelaneNet")}</summary>
          <label>
            {t("setup.controlServiceUrl")}
            <input
              name="server"
              type="url"
              required
              defaultValue="https://room.nodelane.net"
              maxLength={2048}
            />
          </label>
        </details>
        <button className="primary" disabled={!!busy}>
          {t("setup.startYourJourney")}
        </button>
        <p className="hint">{t("setup.identityHelp")}</p>
      </form>
      <Account actions={actions} />
    </section>
  );
}
