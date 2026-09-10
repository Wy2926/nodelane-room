import { useEffect, useRef, useState } from "react";
import { isTauri } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { copyText, exitApp, failure, rpc } from "../native/api";
import type { Failure, Request } from "../shared/model";
import type { Dialog } from "../features/rooms/dialogs/types";
export function useActions(refreshAll: () => void) {
  const [dialog, setDialog] = useState<Dialog>();
  const [busy, setBusy] = useState("");
  const busyRef = useRef(false);
  const [error, setError] = useState<Failure>();
  const [notice, setNotice] = useState("");
  useEffect(() => {
    if (!isTauri()) return;
    const subscription = listen("leave-and-exit", () => {
      if (busyRef.current) return;
      setDialog({
        type: "confirm",
        title: "离房并退出",
        description: "停止本机联机后退出界面。房主离房会保留房间管理权。",
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
      const t = setTimeout(() => setNotice(""), 4000);
      return () => clearTimeout(t);
    }
  }, [notice]);
  async function perform<T>(
    label: string,
    request: Request,
    success?: (value: T) => void,
  ) {
    if (busyRef.current) return false;
    busyRef.current = true;
    setBusy(label);
    setError(undefined);
    setNotice("");
    try {
      const result = await rpc<T>(request);
      success?.(result);
      refreshAll();
      return true;
    } catch (e) {
      setError(failure(e));
      refreshAll();
      return false;
    } finally {
      busyRef.current = false;
      setBusy("");
    }
  }

  async function copy(value: string) {
    try {
      await copyText(value);
      setNotice("已复制");
    } catch {
      setError({
        code: "clipboard",
        error: "无法写入剪贴板，请手动选择并复制。",
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
  };
}
export type Actions = ReturnType<typeof useActions>;
