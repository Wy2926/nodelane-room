import { useCallback, useEffect, useRef, useState } from "react";
import { clientVersion, failure, notifyState, rpc } from "./api";
import type { Failure, Status } from "../shared/model";

export function useService() {
  const [status, setStatus] = useState<Status>();
  const [error, setError] = useState<Failure>();
  const [updatedAt, setUpdatedAt] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  const [retryAt, setRetryAt] = useState(0);
  const [stale, setStale] = useState(false);
  const refreshRef = useRef<() => void>(() => {});
  const refresh = useCallback(() => refreshRef.current(), []);
  useEffect(() => {
    let stopped = false,
      inFlight = false,
      pending = false,
      failures = 0;
    let previous: Status | undefined;
    let timer: ReturnType<typeof setTimeout>;
    let lastResponse = Date.now();
    const watchdog = setInterval(() => {
      if (!stopped) setStale(Date.now() - lastResponse >= 10000);
    }, 1000);
    const poll = async () => {
      if (stopped) return;
      clearTimeout(timer);
      if (inFlight) {
        pending = true;
        return;
      }
      inFlight = true;
      setRefreshing(true);
      try {
        const next = await rpc<Status>({ action: "status" });
        if (next.protocol_version !== 3)
          throw { code: "local_protocol_incompatible" };
        if (next.version !== clientVersion)
          throw { code: "local_version_mismatch" };
        if (stopped) return;
        if (
          previous?.service_instance_id === next.service_instance_id &&
          next.status_seq <= previous.status_seq
        )
          return;
        if (previous?.service_instance_id !== next.service_instance_id)
          previous = undefined;
        if (previous?.engine === "running" && next.engine !== "running")
          void notifyState("disconnected");
        if (previous?.game?.enabled && next.game?.enabled === false)
          void notifyState("disabled");
        if (
          previous &&
          previous.control !== "offline" &&
          next.control === "offline"
        )
          void notifyState("control_unavailable");
        previous = next;
        failures = 0;
        lastResponse = Date.now();
        setStale(false);
        setStatus(next);
        setError(undefined);
        setUpdatedAt(Date.now());
      } catch (e) {
        if (!stopped) {
          failures++;
          setError(failure(e));
        }
      } finally {
        inFlight = false;
        if (!stopped) {
          setRefreshing(false);
          const delay = pending ? 0 : Math.min(15000, 2000 * 2 ** Math.min(failures, 3));
          setRetryAt(Date.now() + delay);
          timer = setTimeout(
            poll,
            delay,
          );
          pending = false;
        }
      }
    };
    refreshRef.current = () => {
      void poll();
    };
    void poll();
    return () => {
      stopped = true;
      clearTimeout(timer);
      clearInterval(watchdog);
    };
  }, []);
  return { status, error, refresh, updatedAt, refreshing, retryAt, stale };
}
