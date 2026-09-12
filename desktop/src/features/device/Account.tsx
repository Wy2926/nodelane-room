import { useState } from "react";
import {
  ArrowRight,
  ArrowSquareOut,
  Copy,
  Desktop,
  UserCircle,
} from "@phosphor-icons/react";
import { t } from "../../i18n";
import { useAccount } from "./use-account";
import { PlayerAvatar } from "../../shared/ui/PlayerAvatar";
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
  const account = useAccount(status, actions, unavailable);
  const user = status?.user;
  const { devices, occupancy } = account;
  return (
    <section className="account-panel" aria-label={t("account.title")}>
      <div className="account-profile">
        {user ? (
          <PlayerAvatar name={user.name} />
        ) : (
          <UserCircle size={52} weight="light" aria-hidden="true" />
        )}
        <div className="account-profile-copy">
          <h3>{user?.name || t("account.signedOut")}</h3>
          <p>
            {t(
              user
                ? user.kind === "guest"
                  ? "account.guest"
                  : "account.registered"
                : "account.loginHelp",
            )}
          </p>
        </div>
      </div>
      {user?.kind === "guest" && (
        <p className="account-description">{t("account.guestHelp")}</p>
      )}
      <AccountAccess {...{ status, actions, account, unavailable }} />
      {user?.kind === "registered" &&
        occupancy &&
        occupancy.device_id !== status?.device_id && (
          <div className="account-occupancy">
            <p>{t("account.otherDeviceConnected")}</p>
            <button
              disabled={unavailable || !!actions.busy || account.waiting}
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
          </div>
        )}
      {user?.kind === "registered" && devices.length > 0 && (
        <details className="account-disclosure">
          <summary>
            {t("interaction.devices")}
            <span className="account-count">
              {devices.filter((device) => !device.revoked).length}
            </span>
          </summary>
          <ul className="account-devices">
            {devices.map((device) => (
              <li key={device.device_id}>
                <Desktop size={23} weight="light" aria-hidden="true" />
                <div>
                  <strong>{device.name}</strong>
                  <small>
                    {t(
                      device.revoked
                        ? "account.deviceRevoked"
                        : device.device_id === status?.device_id
                          ? "account.currentDevice"
                          : "account.authorizedDevice",
                    )}
                  </small>
                </div>
                {!device.revoked && device.device_id !== status?.device_id && (
                  <button
                    className="text-button danger"
                    disabled={unavailable || !!actions.busy || account.waiting}
                    onClick={() =>
                      actions.confirm(
                        t("interaction.revokeDevice"),
                        device.name,
                        {
                          action: "revoke-device",
                          target: device.device_id,
                        },
                      )
                    }
                  >
                    {t("interaction.revokeDevice")}
                  </button>
                )}
              </li>
            ))}
          </ul>
        </details>
      )}
      {user && (
        <details className="account-disclosure">
          <summary>{t("account.details")}</summary>
          <dl className="device-details">
            <div>
              <dt>{t("account.id")}</dt>
              <dd className="device-identity">
                <span className="mono selectable">{user.id}</span>
                <button
                  className="icon-button"
                  aria-label={t("account.copyId")}
                  title={t("account.copyId")}
                  onClick={() => void actions.copy(user.id)}
                >
                  <Copy size={18} aria-hidden="true" />
                </button>
              </dd>
            </div>
          </dl>
        </details>
      )}
    </section>
  );
}

export function AccountAccess({
  status,
  actions,
  account,
  unavailable = false,
}: {
  status?: Status;
  actions: Actions;
  account: ReturnType<typeof useAccount>;
  unavailable?: boolean;
}) {
  const guest = status?.user?.kind === "guest";
  const registered = status?.user?.kind === "registered";
  const [loginAction, setLoginAction] = useState<
    "account-login" | "account-link"
  >(guest ? "account-link" : "account-login");
  const { waiting, loginAvailable, state, message, server } = account;
  const blocked = unavailable || !!actions.busy;
  function login() {
    setLoginAction("account-login");
    const request = {
      action: "account-login" as const,
      server,
      name: status?.name || t("account.defaultName"),
    };
    if (status?.device_id && status.identity !== "signed_out")
      actions.confirm(
        t(registered ? "account.switch" : "account.login"),
        t("account.switchHelp"),
        request,
      );
    else void actions.perform(t("account.signInOrRegister"), request);
  }
  return (
    <div className="account-access">
      {waiting ? (
        <div className="account-waiting">
          <ArrowSquareOut size={25} weight="light" aria-hidden="true" />
          <p role="status">{t("account.waiting")}</p>
          <div className="account-actions">
            <button
              disabled={blocked}
              onClick={() =>
                void actions.perform(t("account.reopen"), {
                  action: loginAction,
                  server,
                  name: status?.name || t("account.defaultName"),
                })
              }
            >
              {t("account.reopen")}
            </button>
            <button
              className="text-button"
              disabled={blocked}
              onClick={() =>
                void actions.perform(t("account.cancel"), {
                  action: "account-cancel",
                })
              }
            >
              {t("account.cancel")}
            </button>
          </div>
        </div>
      ) : (
        <div className="account-actions">
          {guest ? (
            <>
              <button
                className="primary"
                disabled={blocked || !loginAvailable}
                onClick={() => {
                  setLoginAction("account-link");
                  void actions.perform(t("account.bind"), {
                    action: "account-link",
                  });
                }}
              >
                {t("account.bind")}
                <ArrowRight size={18} aria-hidden="true" />
              </button>
              <button
                className="text-button"
                disabled={blocked || !loginAvailable}
                onClick={login}
              >
                {t("account.login")}
              </button>
            </>
          ) : (
            <button
              className={registered ? "" : "primary"}
              disabled={blocked || !loginAvailable}
              onClick={login}
            >
              {t(registered ? "account.switch" : "account.signInOrRegister")}
              {!registered && <ArrowRight size={18} aria-hidden="true" />}
            </button>
          )}
          {registered && (
            <button
              className="text-button danger"
              disabled={blocked}
              onClick={() =>
                actions.confirm(t("account.logout"), t("account.logoutHelp"), {
                  action: "account-logout",
                })
              }
            >
              {t("account.logout")}
            </button>
          )}
        </div>
      )}
      {state === "conflict" ? (
        <p className="hint" role="alert">
          {t("account.conflict")}
        </p>
      ) : ["expired", "failed"].includes(state) ? (
        <p className="hint" role="alert">
          {message || t("account.retry")}
        </p>
      ) : (
        message && (
          <p className="hint" role="status">
            {message}
          </p>
        )
      )}
    </div>
  );
}
