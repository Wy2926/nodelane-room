import { useEffect, useRef } from "react";
import { t } from "../../i18n";
import { defaultServer, failure } from "../../native/api";
import { useQuery } from "../../native/use-query";
import type { Actions } from "../../app/use-actions";
import type { Status } from "../../shared/model";

export function useAccount(
  status: Status | undefined,
  actions: Actions,
  unavailable: boolean,
) {
  const server = status?.server || defaultServer;
  const options = {
    enabled: !unavailable,
    scope: `${status?.service_instance_id}:${status?.user?.id}:${status?.user?.kind}`,
  };
  const capabilities = useQuery<{ oidc_enabled: boolean; ready: boolean }>(
    { action: "capabilities", server },
    0,
    options,
  );
  const registered = {
    ...options,
    enabled: !unavailable && status?.user?.kind === "registered",
  };
  const devices = useQuery<
    { device_id: string; name: string; revoked: boolean }[]
  >({ action: "account-devices" }, 7000, registered);
  const account = useQuery<{
    occupancy?: { room_id: string; device_id: string; revision: number };
  }>({ action: "account-status" }, 7000, registered);
  const login = useQuery<{ state: string; code?: string }>(
    { action: "account-poll" },
    2000,
    options,
  );
  const actionsRef = useRef(actions);
  actionsRef.current = actions;
  useEffect(() => {
    if (login.data?.state === "ready")
      void actionsRef.current.perform(t("account.completed"), {
        action: "status",
      });
  }, [login.data]);
  const state = login.data?.state || "none";
  const error =
    login.error || devices.error || account.error || capabilities.error;
  const code =
    (login.data?.code !== "ok" && login.data?.code) ||
    (capabilities.data && !capabilities.data.oidc_enabled
      ? "auth_oidc_unavailable"
      : undefined);
  return {
    server,
    state,
    waiting: [
      "starting",
      "waiting",
      "pending",
      "authorizing",
      "exchanging",
      "verified",
      "leaving_old_room",
      "saving_identity",
    ].includes(state),
    message: error
      ? failure(error).error
      : code
        ? failure({ code }).error
        : state === "ready"
          ? t("account.completed")
          : "",
    loginAvailable:
      !!capabilities.data?.oidc_enabled && capabilities.data.ready,
    devices: devices.data || [],
    occupancy: account.data?.occupancy,
  };
}
