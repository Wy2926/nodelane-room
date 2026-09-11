import { t, type MessageKey } from "../i18n";
import { invoke, isTauri } from "@tauri-apps/api/core";
import { writeText } from "@tauri-apps/plugin-clipboard-manager";
import type { Request, Failure } from "../shared/model";
import { version } from "../../package.json";

export const clientVersion = version;

export function rpc<T>(request: Request): Promise<T> {
  if (!isTauri())
    return Promise.reject({
      code: "desktop_required",
      error:
        t("api.desktopRequired"),
    });
  return invoke<T>("player_request", { requestData: request });
}

const descriptions: Record<string, MessageKey> = {
  desktop_required: "api.desktopRequired",
  control_unavailable: "api.controlUnavailable",
  invalid_response: "api.invalidResponse",
  busy: "api.busy",
  not_found: "api.notFound",
  unauthorized: "api.unauthorized",
  operation_failed: "api.operationFailed",
  clipboard: "useActions.clipboardError",
  autostart: "settings.autostartError",
  service_unavailable:
    "api.serviceUnavailable",
  permission_denied:
    "api.permissionDenied",
  incompatible: "api.incompatible",
  version_mismatch: "api.versionMismatch",
  forbidden: "api.forbidden",
  conflict: "api.conflict",
  invalid_request: "api.invalidRequest",
  unconfigured: "api.unconfigured",
  configured_ports: "api.configuredPorts",
  rate_limited: "api.rateLimited",
  timeout: "api.timeout",
  update_required: "updates.required",
  update_install_failed: "updates.installFailed",
  update_not_ready: "updates.notReady",
  update_install_unsupported: "updates.unsupported",
};

export function failure(value: unknown): Failure {
  if (
    value &&
    typeof value === "object" &&
    "code" in value &&
    "error" in value
  ) {
    const e = value as Failure;
    return {
      code: String(e.code),
      error: descriptions[e.code] ? t(descriptions[e.code]) : String(e.error).slice(0, 500),
    };
  }
  return { code: "operation_failed", error: t("api.operationFailed") };
}

export const copyText = (text: string) => writeText(text);
export const exitApp = () => invoke("exit_app");
export const notifyState = (kind: string) =>
  invoke("notify_state", { kind }).catch(() => undefined);
