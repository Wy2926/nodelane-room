import { useEffect, useState } from "react";
import { t } from "../../i18n";
import { failure, rpc } from "../../native/api";
import type { Actions } from "../../app/use-actions";
import type { Status } from "../../shared/model";

export function Account({ status, actions }: { status?: Status; actions: Actions }) {
  const [server, setServer] = useState("https://room.nodelane.net");
  const [name, setName] = useState("");
  const [state, setState] = useState("none");
  const [message, setMessage] = useState("");
  const configured = !!status?.device_id;
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const result = await rpc<{ state: string }>({ action: "account-poll" });
        if (!active) return;
        setState(result.state || "none");
        if (result.state === "ready") {
          setMessage(t("account.completed"));
          await actions.perform(t("account.completed"), { action: "status" });
        }
      } catch (e) {
        if (active) setMessage(failure(e).error);
      }
      if (active) timer = setTimeout(poll, 2000);
    }
    void poll();
    return () => { active = false; clearTimeout(timer); };
    // The account transaction is owned by the Go service; resume after remounts.
  }, []);
  const waiting = ["pending", "authorizing", "exchanging", "verified"].includes(state);
  function login() {
    const request = { action: "account-login" as const, server, name: name.trim() || status?.name || t("account.defaultName") };
    if (configured && status?.control !== "signed_out") actions.confirm(t("account.login"), t("account.switchHelp"), request);
    else void actions.perform(t("account.login"), request);
  }
  return <section className="account-panel" aria-label={t("account.title")}>
    <h3>{t("account.title")}</h3>
    {status?.user ? <p><strong>{status.user.name}</strong> · {t(status.user.kind === "guest" ? "account.guest" : "account.registered")}<br /><span className="mono selectable">{status.user.id}</span></p> : <p>{t("account.loginHelp")}</p>}
    {status?.user?.kind === "guest" && <p>{t("account.guestHelp")}</p>}
    {!configured && <details><summary>{t("account.loginOptions")}</summary><label>{t("settings.gamingService")}<input value={server} type="url" maxLength={2048} onChange={e => setServer(e.target.value)} /></label><label>{t("account.newName")}<input value={name} maxLength={40} onChange={e => setName(e.target.value)} /></label></details>}
    <div className="account-actions">
      {status?.user?.kind === "guest" && <button disabled={!!actions.busy || waiting} onClick={() => void actions.perform(t("account.bind"), { action: "account-link" })}>{t("account.bind")}</button>}
      <button disabled={!!actions.busy || waiting} onClick={login}>{t("account.login")}</button>
      {status?.user?.kind === "registered" && <>
        <button disabled={!!actions.busy} onClick={() => actions.confirm(t("account.takeover"), t("account.takeoverHelp"), { action: "account-takeover" })}>{t("account.takeover")}</button>
        <button disabled={!!actions.busy} onClick={() => actions.confirm(t("account.logout"), t("account.logoutHelp"), { action: "account-logout" })}>{t("account.logout")}</button>
      </>}
      {waiting && <button disabled={!!actions.busy} onClick={() => void actions.perform(t("account.cancel"), { action: "account-cancel" })}>{t("account.cancel")}</button>}
    </div>
    {waiting && <p role="status">{t("account.waiting")}</p>}
    {state === "conflict" && <p role="alert">{t("account.conflict")}</p>}
    {["expired", "failed"].includes(state) && <p role="alert">{t("account.retry")}</p>}
    {message && <p role="status">{message}</p>}
  </section>;
}
