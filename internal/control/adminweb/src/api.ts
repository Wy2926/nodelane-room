import type { API } from "./types";

export class APIError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
export function makeAPI(csrf: string, onUnauthorized: () => void): API {
  return async <T>(
    path: string,
    body?: unknown,
    method = body === undefined ? "GET" : "POST",
    signal?: AbortSignal,
  ): Promise<T> => {
    const headers: Record<string, string> = {
      "Content-Type": body instanceof Blob ? "application/octet-stream" : "application/json",
    };
    if (method !== "GET") {
      headers["X-CSRF-Token"] = csrf;
      headers["Idempotency-Key"] = crypto.randomUUID().replaceAll("-", "");
    }
    const res = await fetch("/v2/admin" + path, {
      method,
      headers,
      body: body === undefined ? undefined : body instanceof Blob ? body : JSON.stringify(body),
      signal,
    });
    const data = await res.json().catch(() => {
      throw new Error("控制端响应无法读取，请检查服务和反代。");
    });
    if (!res.ok) {
      if (res.status === 401 && path !== "/login") onUnauthorized();
      const messages: Record<string, string> = {
        unauthorized: "账号或密码错误，或会话已失效。",
        forbidden: "操作未获授权，请检查当前状态。",
        conflict: "状态已变化，请刷新后重试。",
        rate_limited: "操作过于频繁，请稍后重试。",
      };
      throw new APIError(
        messages[data.code] || data.message || "操作失败",
        res.status,
      );
    }
    return data as T;
  };
}
