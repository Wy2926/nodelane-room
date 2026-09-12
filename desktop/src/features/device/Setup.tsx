import { useState } from "react";
import { ArrowLeft, ArrowRight } from "@phosphor-icons/react";
import { AccountAccess } from "./Account";
import { useAccount } from "./use-account";
import { t } from "../../i18n";
import { defaultServer } from "../../native/api";
import brandMark from "../../assets/brand-mark.png";
import type { Actions } from "../../app/use-actions";
import type { Status } from "../../shared/model";

export function Setup({
  actions,
  status,
}: {
  actions: Actions;
  status: Status;
}) {
  const [guest, setGuest] = useState(false);
  const account = useAccount(status, actions, false);
  const blocked = !!actions.busy || account.waiting;
  return (
    <section className="welcome" aria-labelledby="welcome-title">
      {!guest && <img className="welcome-mark" src={brandMark} alt="" />}
      <h2 id="welcome-title">
        {t(guest ? "setup.guestTitle" : "setup.welcomeToNodelane")}
      </h2>
      <p className="muted">
        {t(guest ? "setup.nicknameHelp" : "setup.loginHelp")}
      </p>
      {guest ? (
        <>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              if (blocked) return;
              const name = String(
                new FormData(e.currentTarget).get("name"),
              ).trim();
              if (!name) return;
              if (new TextEncoder().encode(name).length > 80) {
                actions.setError({
                  code: "request_validation_failed",
                  error: t("interaction.byteLimit"),
                });
                return;
              }
              void actions.perform(t("setup.initializingDevice"), {
                action: "init",
                server: defaultServer,
                name,
              });
            }}
          >
            <label>
              {t("setup.nickname")}
              <input
                name="name"
                required
                maxLength={40}
                autoFocus
                autoComplete="nickname"
                placeholder={t("setup.aNameYourFriendsWillRecognize")}
                disabled={blocked}
              />
            </label>
            <button className="primary full" disabled={blocked}>
              {t("setup.startYourJourney")}
              <ArrowRight size={18} aria-hidden="true" />
            </button>
          </form>
          <p className="hint welcome-note">{t("setup.guestHelp")}</p>
          <button
            className="text-button"
            disabled={blocked}
            onClick={() => setGuest(false)}
          >
            <ArrowLeft size={16} aria-hidden="true" />
            {t("setup.backToLogin")}
          </button>
        </>
      ) : (
        <>
          <AccountAccess account={account} actions={actions} />
          {!account.waiting && (
            <button
              className="text-button welcome-guest"
              disabled={blocked}
              onClick={() => setGuest(true)}
            >
              {t("setup.continueAsGuest")}
              <ArrowRight size={16} aria-hidden="true" />
            </button>
          )}
        </>
      )}
    </section>
  );
}
