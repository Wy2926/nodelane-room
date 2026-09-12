import { t, type MessageKey } from "../i18n";
import { invoke, isTauri } from "@tauri-apps/api/core";
import { writeText } from "@tauri-apps/plugin-clipboard-manager";
import type { Request, Failure, Envelope } from "../shared/model";
import zhCN from "../i18n/locales/zh-CN.json";
import { version } from "../../package.json";

export const clientVersion = version;
export const defaultServer = "https://room.nodelane.net";
export const isKnownCode = (code: string) =>
  Object.hasOwn(zhCN, `business.${code}`);

export async function rpc<T>(request: Request): Promise<T> {
  if (!isTauri())
    return Promise.reject({
      code: "local_desktop_required",
      error: t("api.desktopRequired"),
    });
  const result = await invoke<Envelope<T>>("player_request", {
    requestData: {
      ...request,
      contract: "interaction-1",
      command_id: request.command_id || crypto.randomUUID().replaceAll("-", ""),
    },
  });
  if (
    !result ||
    result.contract !== "interaction-1" ||
    !result.request_id ||
    !result.code
  )
    throw { code: "local_ipc_response_invalid" };
  if (!["ok", "operation_noop", "admin_password_changed"].includes(result.code))
    throw result;
  const data = result.data as unknown;
  const record =
    data && typeof data === "object" && !Array.isArray(data)
      ? (data as Record<string, unknown>)
      : undefined;
  const valid =
    request.action === "status"
      ? record &&
        typeof record.service_instance_id === "string" &&
        typeof record.status_seq === "number" &&
        record.membership &&
        record.permissions &&
        record.network &&
        Array.isArray(record.pending_operations)
      : request.action === "rooms"
        ? record &&
          Array.isArray(record.rooms) &&
          typeof record.truncated === "boolean"
        : request.action === "get-operation"
          ? record &&
            typeof record.id === "string" &&
            [
              "submitting",
              "pending",
              "reconciling",
              "succeeded",
              "rejected",
              "unresolved",
            ].includes(String(record.state))
          : request.action === "games"
            ? Array.isArray(data)
            : true;
  if (!valid)
    throw {
      code: "local_ipc_response_invalid",
      request_id: result.request_id,
      operation_id: result.operation_id,
    };
  return result.data;
}

export function failure(value: unknown): Failure {
  if (value && typeof value === "object" && "code" in value) {
    const e = value as Failure;
    const key = `business.${e.code}` as MessageKey;
    return {
      code: String(e.code),
      error: Object.hasOwn(zhCN, key) ? t(key) : t("api.unsupportedResult"),
      request_id: e.request_id,
      operation_id: e.operation_id,
      origin: e.origin,
      control_http_status: e.control_http_status,
      cause_request_id: e.cause_request_id,
      details: e.details,
      retry: e.retry,
    };
  }
  return { code: "local_internal_error", error: t("api.operationFailed") };
}

export const copyText = (text: string) => writeText(text);
export const exitApp = () => invoke("exit_app");
export const notifyState = (kind: string) =>
  invoke("notify_state", { kind }).catch(() => undefined);
