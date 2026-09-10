import { useCallback, useEffect, useRef, useState } from "react";
import { clientVersion, failure, notifyState, rpc } from "./api";
import type { Failure, Status } from "../shared/model";

export function useService() {
  const [status, setStatus] = useState<Status>();
  const [error, setError] = useState<Failure>();
  const [updatedAt, setUpdatedAt] = useState(0);
  const refreshRef = useRef<() => void>(() => {});
  const refresh = useCallback(() => refreshRef.current(), []);
  useEffect(() => {
    let stopped = false,
      inFlight = false,
      pending = false,
      failures = 0;
    let previous: Status | undefined;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      if (stopped) return;
      clearTimeout(timer);
      if (inFlight) {
        pending = true;
        return;
      }
      inFlight = true;
      try {
        const next = await rpc<Status>({ action: "status" });
        if (next.protocol_version !== 1)
          throw { code: "incompatible", error: "protocol mismatch" };
        if (next.version !== clientVersion)
          throw { code: "version_mismatch", error: "version mismatch" };
        if (stopped) return;
        if (previous?.engine === "running" && next.engine !== "running")
          void notifyState("disconnected");
        if (previous?.game?.enabled && next.game?.enabled === false)
          void notifyState("disabled");
        if (
          previous &&
          previous.control !== "unreachable" &&
          next.control === "unreachable"
        )
          void notifyState("control_unavailable");
        previous = next;
        failures = 0;
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
          timer = setTimeout(
            poll,
            pending ? 0 : Math.min(15000, 2000 * 2 ** Math.min(failures, 3)),
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
    };
  }, []);
  return { status, error, refresh, updatedAt };
}
