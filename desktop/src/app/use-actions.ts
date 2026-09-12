import { t } from "../i18n";
import { useEffect, useRef, useState } from "react";
import { isTauri } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { copyText, exitApp, failure, isKnownCode, rpc } from "../native/api";
import type { Failure, Request, Operation, Status } from "../shared/model";
import { useOperations } from "./use-operations";
import type { Dialog } from "../features/rooms/dialogs/types";
const roomActions = new Set([
  "create",
  "join",
  "owner-join",
  "invite",
  "invite-revoke",
  "kick",
  "transfer",
  "leave",
  "close",
]);
const writes = new Set([
  ...roomActions,
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
  const scope = useRef({});
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
  const [dialog, setDialog] = useState<Dialog>();
  const [busy, setBusyState] = useState("");
  const busyRef = useRef(false);
  const [error, setError] = useState<Failure>();
  const [notice, setNotice] = useState(false);
  const pauseRef = useRef<object>(undefined);
  const [pausing, setPausing] = useState(false);
  const [retryAt, setRetryAt] = useState(0);
  const [now, setNow] = useState(Date.now());
  const { hasPending, track, followup, ...operations } = useOperations({
    instance,
    operations: options?.operations,
    unavailable: options?.unavailable,
    busy: busyRef,
    refresh: refreshAll,
    setError,
    closeDialog: () => setDialog(undefined),
  });
  function setBusy(label: string) {
    busyRef.current = !!label;
    setBusyState(label);
  }
  useEffect(() => {
    scope.current = {};
    resumeIntent.current = undefined;
    setTakeover(undefined);
    setDialog(undefined);
    setBusy("");
    pauseRef.current = undefined;
    setPausing(false);
    setError(undefined);
    setRetryAt(0);
    return () => {
      scope.current = {};
      pauseRef.current = undefined;
    };
  }, [instance]);
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
    const sourceScope = scope.current;
    if (request.action === "network-stop") {
      if (pauseRef.current) return false;
      const ticket = {};
      pauseRef.current = ticket;
      setPausing(true);
      try {
        await rpc(request);
        if (scope.current !== sourceScope) return false;
        refreshAll();
        return true;
      } catch (e) {
        if (scope.current === sourceScope) setError(failure(e));
        return false;
      } finally {
        if (pauseRef.current === ticket) {
          pauseRef.current = undefined;
          setPausing(false);
        }
      }
    }
    if (optionsRef.current?.unavailable) {
      setError(failure({ code: "local_service_unavailable" }));
      return false;
    }
    if (optionsRef.current?.roomBlocked && roomActions.has(request.action)) {
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
    if (busyRef.current || pauseRef.current) return false;
    if (
      hasPending() &&
      !["get-operation", "network-retry", "ping", "doctor"].includes(
        request.action,
      )
    )
      return false;
    setBusy(label);
    setError(undefined);
    setNotice(false);
    try {
      if (writes.has(request.action))
        request = {
          ...request,
          command_id:
            request.command_id || crypto.randomUUID().replaceAll("-", ""),
        };
      const result = await rpc<T>(request);
      if (scope.current !== sourceScope) {
        refreshAll();
        return false;
      }
      success?.(result);
      refreshAll();
      return true;
    } catch (e) {
      const issue = failure(e);
      if (scope.current !== sourceScope) return false;
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
        track(
          issue.operation_id || request.command_id!,
          success as ((value: unknown) => void) | undefined,
        );
      }
      refreshAll();
      return false;
    } finally {
      if (scope.current === sourceScope) {
        setBusy("");
      }
    }
  }

  async function takeOverAndContinue() {
    const intent = resumeIntent.current;
    const target = takeover;
    if (!intent || !target) return;
    const sourceScope = scope.current;
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
    if (scope.current !== sourceScope) return;
    if (accepted) await resume();
    else followup(resume);
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
    pausing,
    setBusy,
    error,
    setError,
    notice,
    perform,
    copy,
    confirm,
    quit,
    ...operations,
    retrySeconds: Math.max(0, Math.ceil((retryAt - now) / 1000)),
    takeover,
    takeOverAndContinue,
    cancelTakeover: () => {
      setTakeover(undefined);
      resumeIntent.current = undefined;
    },
  };
}
export type Actions = ReturnType<typeof useActions>;
