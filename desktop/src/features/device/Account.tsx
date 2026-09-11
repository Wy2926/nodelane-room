import { useEffect, useState } from "react";
import { t } from "../../i18n";
import { failure, rpc } from "../../native/api";
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
  const [server, setServer] = useState("https://room.nodelane.net");
  const [name, setName] = useState("");
  const [state, setState] = useState("none");
  const [message, setMessage] = useState("");
  const [loginAvailable, setLoginAvailable] = useState(false);
  useEffect(() => {
    if (unavailable) return;
    let active = true;
    setLoginAvailable(false);
    const timer = setTimeout(() => {
      void rpc<{ oidc_enabled: boolean; ready: boolean }>({
        action: "capabilities",
        server,
      })
        .then((value) => {
          if (active) {
            setLoginAvailable(value.oidc_enabled && value.ready);
            if (!value.oidc_enabled)
              setMessage(failure({ code: "auth_oidc_unavailable" }).error);
          }
        })
        .catch((e) => {
          if (active) setMessage(failure(e).error);
        });
    }, 300);
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [server, status?.service_instance_id, unavailable]);
  const [devices, setDevices] = useState<
    { device_id: string; name: string; revoked: boolean }[]
  >([]);
  const [occupancy, setOccupancy] = useState<{
    room_id: string;
    device_id: string;
    revision: number;
  }>();
  const configured = !!status?.device_id;
  useEffect(() => {
    if (unavailable) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const [list, me] = await Promise.all([
          rpc<typeof devices>({ action: "account-devices" }),
          rpc<{ occupancy?: typeof occupancy }>({ action: "account-status" }),
        ]);
        if (active) {
          setDevices(list);
          setOccupancy(me.occupancy);
        }
      } catch (e) {
        if (active) setMessage(failure(e).error);
      }
      if (active) timer = setTimeout(poll, 7000);
    }
    if (status?.user?.kind === "registered") void poll();
    else {
      setDevices([]);
      setOccupancy(undefined);
    }
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [status?.user?.id, status?.service_instance_id, unavailable]);
  useEffect(() => {
    if (unavailable) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const result = await rpc<{ state: string; code?: string }>({
          action: "account-poll",
        });
        if (!active) return;
        setState(result.state || "none");
        if (result.code && result.code !== "ok")
          setMessage(failure({ code: result.code }).error);
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
    return () => {
      active = false;
      clearTimeout(timer);
    };
    // The account transaction is owned by the Go service; resume after remounts.
  }, [unavailable]);
  const waiting = [
    "starting",
    "waiting",
    "pending",
    "authorizing",
    "exchanging",
    "verified",
    "leaving_old_room",
    "saving_identity",
  ].includes(state);
  function login() {
    if (new TextEncoder().encode(name.trim()).length > 80) {
      setMessage(t("interaction.byteLimit"));
      return;
    }
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
            {t("settings.gamingService")}
            <input
              value={server}
              type="url"
              maxLength={2048}
              onChange={(e) => setServer(e.target.value)}
            />
          </label>
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
      {message && <p role="status">{message}</p>}
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
