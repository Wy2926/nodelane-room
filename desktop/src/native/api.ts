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
        "请打开已安装的 NodeLane Room 桌面应用。本网页预览不连接本机网络服务。",
    });
  return invoke<T>("player_request", { requestData: request });
}

const descriptions: Record<string, string> = {
  service_unavailable:
    "网络后台不可用。请完成安装并确认 NodeLane 服务正在运行。",
  permission_denied:
    "当前系统账户未获授权。请使用安装时指定的玩家账户打开应用。",
  incompatible: "客户端与后台协议不兼容，请使用同一安装包中的程序。",
  version_mismatch: "客户端与后台版本不一致。请退出界面并重新打开；若仍不一致，请重新运行完整安装包。",
  forbidden: "操作未获授权。房间权限或游戏配置可能已改变，请刷新后查看。",
  conflict: "操作与当前房间状态冲突，请刷新后查看。",
  invalid_request: "输入不符合要求，请检查后重试。",
  unconfigured: "请先配置控制端和设备昵称。",
  configured_ports: "此游戏的端口由服务端管理，请使用游戏配置中的端口。",
  rate_limited: "请求过于频繁，请稍后再试。",
  timeout: "请求超时，操作结果尚未确认。请先刷新状态，避免重复操作。",
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
      error: descriptions[e.code] || String(e.error).slice(0, 500),
    };
  }
  return { code: "operation_failed", error: "操作未完成，请刷新状态后重试。" };
}

export const copyText = (text: string) => writeText(text);
export const exitApp = () => invoke("exit_app");
export const notifyState = (kind: string) =>
  invoke("notify_state", { kind }).catch(() => undefined);
