import { t } from "../i18n";
import { useEffect, useRef, useState } from "react";
import { isTauri } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { copyText, exitApp, failure, isKnownCode, rpc } from "../native/api";
import type { Failure, Request, Operation } from "../shared/model";
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
export function useActions(refreshAll: () => void, instance?: string) {
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
  const pendingRef = useRef<string>(undefined);
  function setPending(value: string | undefined) {
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
    setPending(undefined);
    busyRef.current = false;
    setBusy("");
  }, [instance]);
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
      try {
        await rpc(request);
        refreshAll();
        return true;
      } catch (e) {
        setError(failure(e));
        return false;
      }
    }
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

  async function checkOperation(id = pending) {
    if (!id || busyRef.current) return;
    const sourceInstance = instanceRef.current;
    busyRef.current = true;
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
        if (operation.result?.data != null)
          pendingSuccess.current?.(operation.result.data);
        else setDialog(undefined);
        setPending(undefined);
        setError(undefined);
        pendingSuccess.current = undefined;
      } else if (operation.state === "rejected") {
        pendingFollowup.current = undefined;
        setPending(undefined);
        setError(failure(operation.result));
        pendingSuccess.current = undefined;
      } else if (operation.state === "unresolved") {
        pendingFollowup.current = undefined;
        setPending(undefined);
        setError(failure({ code: "operation_expired" }));
        pendingSuccess.current = undefined;
      } else setPending(id);
      refreshAll();
    } catch (e) {
      if (instanceRef.current === sourceInstance) setError(failure(e));
    } finally {
      if (instanceRef.current === sourceInstance) busyRef.current = false;
    }
    if (pendingFollowup.current && !pendingSuccess.current) {
      const next = pendingFollowup.current;
      pendingFollowup.current = undefined;
      await next();
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
