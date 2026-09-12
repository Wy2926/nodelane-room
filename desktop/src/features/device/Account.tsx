import { useState } from "react";
import { t } from "../../i18n";
import { useAccount } from "./use-account";
import type { Actions } from "../../app/use-actions";
import type { Status } from "../../shared/model";

export function Account({
  status,
  actions,
  unavailable = false,
}: {
  status?: Status;
  actions: Actions;
  unavailable?: boolean;
}) {
  const [name, setName] = useState("");
  const account = useAccount(status, actions, unavailable);
  const { server, state, waiting, loginAvailable, devices, occupancy } =
    account;
  const [message, setMessage] = useState("");
  const configured = !!status?.device_id;
  function login() {
    if (new TextEncoder().encode(name.trim()).length > 80) {
      setMessage(t("interaction.byteLimit"));
      return;
    }
    setMessage("");
    const request = {
      action: "account-login" as const,
      server,
      name: name.trim() || status?.name || t("account.defaultName"),
    };
    if (configured && status?.identity !== "signed_out")
      actions.confirm(t("account.login"), t("account.switchHelp"), request);
    else void actions.perform(t("account.login"), request);
  }
  return (
    <section className="account-panel" aria-label={t("account.title")}>
      <h3>{t("account.title")}</h3>
      {status?.user ? (
        <p>
          <strong>{status.user.name}</strong> ·{" "}
          {t(
            status.user.kind === "guest"
              ? "account.guest"
              : "account.registered",
          )}
          <br />
          <span className="mono selectable">{status.user.id}</span>
        </p>
      ) : (
        <p>{t("account.loginHelp")}</p>
      )}
      {status?.user?.kind === "guest" && <p>{t("account.guestHelp")}</p>}
      {!configured && (
        <details>
          <summary>{t("account.loginOptions")}</summary>
          <label>
            {t("account.newName")}
            <input
              value={name}
              maxLength={80}
              onChange={(e) => setName(e.target.value)}
            />
          </label>
        </details>
      )}
      <div className="account-actions">
        {status?.user?.kind === "guest" && (
          <button
            disabled={
              unavailable || !loginAvailable || !!actions.busy || waiting
            }
            onClick={() =>
              void actions.perform(t("account.bind"), {
                action: "account-link",
              })
            }
          >
            {t("account.bind")}
          </button>
        )}
        <button
          disabled={unavailable || !loginAvailable || !!actions.busy || waiting}
          onClick={login}
        >
          {t("account.login")}
        </button>
        {waiting && (
          <button
            disabled={unavailable || !!actions.busy}
            onClick={() =>
              void actions.perform(t("account.reopen"), {
                action:
                  status?.user?.kind === "guest"
                    ? "account-link"
                    : "account-login",
              })
            }
          >
            {t("account.reopen")}
          </button>
        )}
        {status?.user?.kind === "registered" && (
          <>
            {occupancy && (
              <button
                disabled={unavailable || !!actions.busy}
                onClick={() =>
                  actions.confirm(
                    t("account.takeover"),
                    t("account.takeoverHelp"),
                    {
                      action: "account-takeover",
                      body: {
                        room_id: occupancy.room_id,
                        device_id: occupancy.device_id,
                        expected_revision: occupancy.revision,
                      },
                    },
                  )
                }
              >
                {t("account.takeover")}
              </button>
            )}
            <button
              disabled={unavailable || !!actions.busy}
              onClick={() =>
                actions.confirm(t("account.logout"), t("account.logoutHelp"), {
                  action: "account-logout",
                })
              }
            >
              {t("account.logout")}
            </button>
          </>
        )}
        {waiting && (
          <button
            disabled={unavailable || !!actions.busy}
            onClick={() =>
              void actions.perform(t("account.cancel"), {
                action: "account-cancel",
              })
            }
          >
            {t("account.cancel")}
          </button>
        )}
      </div>
      {waiting && <p role="status">{t("account.waiting")}</p>}
      {state === "conflict" && <p role="alert">{t("account.conflict")}</p>}
      {["expired", "failed"].includes(state) && (
        <p role="alert">{t("account.retry")}</p>
      )}
      {(message || account.message) && (
        <p role="status">{message || account.message}</p>
      )}
      {devices.length > 0 && (
        <details>
          <summary>{t("interaction.devices")}</summary>
          {devices.map((device) => (
            <div key={device.device_id}>
              <span>{device.name}</span>
              <button
                disabled={
                  unavailable ||
                  device.revoked ||
                  device.device_id === status?.device_id ||
                  !!actions.busy
                }
                onClick={() =>
                  actions.confirm(t("interaction.revokeDevice"), device.name, {
                    action: "revoke-device",
                    target: device.device_id,
                  })
                }
              >
                {t("interaction.revokeDevice")}
              </button>
            </div>
          ))}
        </details>
      )}
    </section>
  );
}
