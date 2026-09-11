import { t } from "../i18n";
import { useEffect, useRef, useState } from "react";
import { isTauri } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { copyText, exitApp, failure, isKnownCode, rpc } from "../native/api";
import type { Failure, Request, Operation, Status } from "../shared/model";
import type { Dialog } from "../features/rooms/dialogs/types";
const writes = new Set([
  "create",
  "join",
  "owner-join",
  "invite",
  "invite-revoke",
  "kick",
  "transfer",
  "leave",
  "close",
  "account-takeover",
  "account-logout",
  "revoke-device",
]);
export function useActions(
  refreshAll: () => void,
  instance?: string,
  options?: {
    operations: Operation[];
    unavailable: boolean;
    roomBlocked: boolean;
    roomCreation?: Status["room_creation"];
    updateRequired?: boolean;
  },
) {
  const optionsRef = useRef(options);
  optionsRef.current = options;
  const refreshRef = useRef(refreshAll);
  refreshRef.current = refreshAll;
  const instanceRef = useRef(instance);
  instanceRef.current = instance;
  const [takeover, setTakeover] = useState<{
    room: string;
    device: string;
    revision: number;
  }>();
  const resumeIntent = useRef<{
    label: string;
    request: Request;
    success?: (value: unknown) => void;
  }>(undefined);
  const pendingFollowup = useRef<(() => Promise<unknown>) | undefined>(
    undefined,
  );
  const [dialog, setDialog] = useState<Dialog>();
  const [busy, setBusy] = useState("");
  const busyRef = useRef(false);
  const [error, setError] = useState<Failure>();
  const [notice, setNotice] = useState(false);
  const [pending, setPendingState] = useState<string>();
  const [unresolved, setUnresolved] = useState(false);
  const [reviewed, setReviewed] = useState(false);
  const [checking, setChecking] = useState(false);
  const checkingRef = useRef(false);
  const pauseRef = useRef(false);
  const acknowledged = useRef(new Set<string>());
  const [retryAt, setRetryAt] = useState(0);
  const [now, setNow] = useState(Date.now());
  const pendingRef = useRef<string>(undefined);
  function setPending(value: string | undefined) {
    if (value !== pendingRef.current) setReviewed(false);
    pendingRef.current = value;
    setPendingState(value);
  }
  const pendingSuccess = useRef<((value: unknown) => void) | undefined>(
    undefined,
  );
  useEffect(() => {
    pendingSuccess.current = undefined;
    pendingFollowup.current = undefined;
    resumeIntent.current = undefined;
    setTakeover(undefined);
    setDialog(undefined);
    busyRef.current = false;
    setBusy("");
    setError(undefined);
    setRetryAt(0);
  }, [instance]);
  useEffect(() => {
    const operation = options?.operations.find(
      (op) => !acknowledged.current.has(op.id),
    );
    if (!pendingRef.current && operation) {
      setPending(operation.id);
      setUnresolved(operation.state === "unresolved");
    }
  }, [options?.operations]);
  const checkRef = useRef<() => Promise<void>>(async () => {});
  checkRef.current = () => checkOperation(pendingRef.current);
  useEffect(() => {
    if (!pending || unresolved || options?.unavailable) return;
    const timer = setInterval(() => void checkRef.current(), 3000);
    return () => clearInterval(timer);
  }, [pending, unresolved, options?.unavailable]);
  useEffect(() => {
    if (!retryAt) return;
    const timer = setInterval(() => {
      setNow(Date.now());
      if (Date.now() >= retryAt) setRetryAt(0);
    }, 250);
    return () => clearInterval(timer);
  }, [retryAt]);
  useEffect(() => {
    if (!isTauri()) return;
    const subscription = listen("leave-and-exit", () => {
      if (busyRef.current) return;
      setDialog({
        type: "confirm",
        title: t("useActions.leaveRoomAndExit"),
        description: t("useActions.leaveAndExitHelp"),
        request: { action: "leave" },
        exit: true,
      });
    });
    return () => {
      void subscription.then((unlisten) => unlisten());
    };
  }, []);

  useEffect(() => {
    if (notice) {
      const timer = setTimeout(() => setNotice(false), 4000);
      return () => clearTimeout(timer);
    }
  }, [notice]);
  async function perform<T>(
    label: string,
    request: Request,
    success?: (value: T) => void,
  ) {
    if (request.action === "network-stop") {
      if (pauseRef.current) return false;
      pauseRef.current = true;
      try {
        await rpc(request);
        refreshAll();
        return true;
      } catch (e) {
        setError(failure(e));
        return false;
      } finally {
        pauseRef.current = false;
      }
    }
    if (optionsRef.current?.unavailable) {
      setError(failure({ code: "local_service_unavailable" }));
      return false;
    }
    if (
      optionsRef.current?.roomBlocked &&
      [
        "create",
        "join",
        "owner-join",
        "invite",
        "invite-revoke",
        "kick",
        "transfer",
        "close",
        "leave",
      ].includes(request.action)
    ) {
      setError(failure({ code: "local_network_offline" }));
      return false;
    }
    if (
      optionsRef.current?.updateRequired &&
      ["create", "join", "owner-join", "network-retry"].includes(request.action)
    ) {
      setError(failure({ code: "client_update_required" }));
      return false;
    }
    if (
      request.action === "create" &&
      optionsRef.current &&
      optionsRef.current.roomCreation?.allowed !== true
    ) {
      setError(
        failure({
          code: optionsRef.current.roomCreation?.reason || "local_state_stale",
        }),
      );
      return false;
    }
    if (retryAt > Date.now()) return false;
    if (busyRef.current) return false;
    if (
      pendingRef.current &&
      ![
        "get-operation",
        "network-stop",
        "network-retry",
        "ping",
        "doctor",
      ].includes(request.action)
    )
      return false;
    busyRef.current = true;
    setBusy(label);
    setError(undefined);
    setNotice(false);
    const sourceInstance = instanceRef.current;
    try {
      if (writes.has(request.action))
        request = {
          ...request,
          command_id:
            request.command_id || crypto.randomUUID().replaceAll("-", ""),
        };
      const result = await rpc<T>(request);
      if (instanceRef.current !== sourceInstance) {
        refreshAll();
        return false;
      }
      success?.(result);
      refreshAll();
      return true;
    } catch (e) {
      const issue = failure(e);
      if (instanceRef.current !== sourceInstance) return false;
      if (issue.code === "operation_result_redacted") {
        setDialog(undefined);
        refreshAll();
        return true;
      }
      setError(issue);
      if (issue.retry?.kind === "after") {
        setNow(Date.now());
        setRetryAt(
          Date.now() +
            Math.min(600000, Math.max(1000, issue.retry.after_ms || 5000)),
        );
      }
      if (
        issue.code === "account_in_use" &&
        ["create", "join", "owner-join"].includes(request.action)
      ) {
        const d = issue.details as
          | { room_id?: string; device_id?: string; actual_revision?: number }
          | undefined;
        if (d?.room_id && d.device_id && d.actual_revision) {
          setTakeover({
            room: d.room_id,
            device: d.device_id,
            revision: d.actual_revision,
          });
          resumeIntent.current = {
            label,
            request,
            success: success as ((value: unknown) => void) | undefined,
          };
        }
      }
      if (
        writes.has(request.action) &&
        (!isKnownCode(issue.code) ||
          [
            "operation_pending",
            "local_rpc_timeout",
            "local_control_unreachable",
            "local_dns_failed",
            "local_tls_failed",
            "local_ipc_response_invalid",
            "local_control_response_invalid",
            "local_control_timeout",
            "local_control_connection_lost",
            "local_storage_failed",
            "system_unavailable",
            "system_internal_error",
          ].includes(issue.code))
      ) {
        setPending(issue.operation_id || request.command_id);
        setUnresolved(false);
        pendingSuccess.current =
          (success as ((value: unknown) => void) | undefined) || (() => {});
      }
      refreshAll();
      return false;
    } finally {
      if (instanceRef.current === sourceInstance) {
        busyRef.current = false;
        setBusy("");
      }
    }
  }

  async function checkOperation(id = pendingRef.current) {
    if (
      !id ||
      busyRef.current ||
      checkingRef.current ||
      optionsRef.current?.unavailable
    )
      return;
    const sourceInstance = instanceRef.current;
    checkingRef.current = true;
    setChecking(true);
    try {
      const operation = await rpc<Operation>({
        action: "get-operation",
        target: id,
      });
      if (instanceRef.current !== sourceInstance) {
        refreshAll();
        return;
      }
      if (operation.state === "succeeded") {
        acknowledged.current.add(id);
        if (operation.result?.data != null)
          pendingSuccess.current?.(operation.result.data);
        else setDialog(undefined);
        setPending(undefined);
        setUnresolved(false);
        setError(undefined);
        pendingSuccess.current = undefined;
      } else if (operation.state === "rejected") {
        acknowledged.current.add(id);
        pendingFollowup.current = undefined;
        setPending(undefined);
        setUnresolved(false);
        setError(failure(operation.result));
        pendingSuccess.current = undefined;
      } else if (operation.state === "unresolved") {
        pendingFollowup.current = undefined;
        setPending(id);
        setUnresolved(true);
        setError(failure({ code: "operation_expired" }));
        pendingSuccess.current = undefined;
      } else {
        setPending(id);
        setUnresolved(false);
      }
      refreshAll();
    } catch (e) {
      if (instanceRef.current === sourceInstance) {
        const issue = failure(e);
        setError(issue);
        if (issue.code === "operation_expired") setUnresolved(true);
      }
    } finally {
      checkingRef.current = false;
      setChecking(false);
    }
    if (pendingFollowup.current && !pendingSuccess.current) {
      const next = pendingFollowup.current;
      pendingFollowup.current = undefined;
      await next();
    }
  }
  async function reviewPending() {
    const id = pendingRef.current;
    if (!id || !unresolved || checkingRef.current) return;
    if (reviewed) {
      acknowledged.current.add(id);
      setPending(undefined);
      setUnresolved(false);
      setError(undefined);
      refreshRef.current();
      return;
    }
    checkingRef.current = true;
    setChecking(true);
    const sourceInstance = instanceRef.current;
    try {
      const status = await rpc<import("../shared/model").Status>({
        action: "status",
      });
      if (sourceInstance !== instanceRef.current || pendingRef.current !== id)
        return;
      if (status.control !== "online")
        throw { code: "local_control_unreachable" };
      setDialog(undefined);
      setReviewed(true);
      refreshRef.current();
    } catch (e) {
      setError(failure(e));
    } finally {
      checkingRef.current = false;
      setChecking(false);
    }
  }
  async function takeOverAndContinue() {
    const intent = resumeIntent.current;
    const target = takeover;
    if (!intent || !target) return;
    setTakeover(undefined);
    resumeIntent.current = undefined;
    const resume = () =>
      perform(
        intent.label,
        { ...intent.request, command_id: undefined },
        intent.success,
      );
    const accepted = await perform(t("interaction.takeover"), {
      action: "account-takeover",
      body: {
        room_id: target.room,
        device_id: target.device,
        expected_revision: target.revision,
      },
    });
    if (accepted) await resume();
    else if (pendingSuccess.current !== undefined || pending)
      pendingFollowup.current = resume;
  }

  async function copy(value: string) {
    try {
      await copyText(value);
      setNotice(true);
    } catch {
      setError({
        code: "local_clipboard_failed",
        error: t("useActions.clipboardError"),
      });
    }
  }

  function confirm(
    title: string,
    description: string,
    request: Request,
    exit = false,
  ) {
    setError(undefined);
    setDialog({ type: "confirm", title, description, request, exit });
  }
  async function quit() {
    try {
      await exitApp();
    } catch (e) {
      setError(failure(e));
    }
  }
  return {
    dialog,
    setDialog,
    busy,
    setBusy,
    error,
    setError,
    notice,
    perform,
    copy,
    confirm,
    quit,
    pending,
    unresolved,
    reviewed,
    checking,
    reviewPending,
    retrySeconds: Math.max(0, Math.ceil((retryAt - now) / 1000)),
    checkOperation,
    takeover,
    takeOverAndContinue,
    cancelTakeover: () => {
      setTakeover(undefined);
      resumeIntent.current = undefined;
    },
  };
}
export type Actions = ReturnType<typeof useActions>;
