import { useEffect, useState } from "react";
import { failure, rpc } from "./api";
import type { Failure, Request } from "../shared/model";

// Read queries retain the last good value only within the same identity and request.
export function useQuery<T>(
  request: Request,
  interval: number,
  { enabled = true, scope = "", reload = 0 } = {},
) {
  const key = JSON.stringify([scope, request]);
  const [result, setResult] = useState<{
    key: string;
    data?: T;
    error?: Failure;
    updatedAt: number;
  }>();
  useEffect(() => {
    if (!enabled) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function load() {
      try {
        const data = await rpc<T>(request);
        if (active) setResult({ key, data, updatedAt: Date.now() });
      } catch (error) {
        if (active)
          setResult((previous) => ({
            key,
            error: failure(error),
            data: previous?.key === key ? previous.data : undefined,
            updatedAt: previous?.key === key ? previous.updatedAt : 0,
          }));
      }
      if (active && interval > 0) timer = setTimeout(load, interval);
    }
    void load();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [key, enabled, interval, reload]);
  const current = result?.key === key ? result : undefined;
  return {
    data: current?.data,
    error: current?.error,
    updatedAt: current?.updatedAt || 0,
    loading: enabled && !current,
  };
}
