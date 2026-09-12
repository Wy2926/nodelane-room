import { useEffect, useState } from "react";
import { invoke, isTauri } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";

// Keep the pending invitation in memory while the first-run/account screens run.
export function useInvitation() {
  const [value, setValue] = useState({invitation: "", revision: 0});
  useEffect(() => {
    if (!isTauri()) return;
    let stopped = false;
    let reading = false;
    let requested = false;
    let unlisten: (() => void) | undefined;
    async function take() {
      if (stopped) return;
      requested = true;
      if (reading) return;
      reading = true;
      try {
        while (requested && !stopped) {
          requested = false;
          let code: string | null;
          try { code = await invoke<string | null>("take_invitation"); }
          catch { continue; }
          if (!stopped && code && /^[a-f0-9]{32}$/.test(code))
            setValue(previous => ({invitation: code, revision: previous.revision + 1}));
        }
      } finally { reading = false; }
    }
    void listen("invitation-ready", () => { void take().catch(() => undefined); })
      .then((off) => {
        if (stopped) { off(); return; }
        unlisten = off;
        void take().catch(() => undefined);
      }).catch(() => undefined);
    return () => { stopped = true; unlisten?.(); };
  }, []);
  return { ...value, clearInvitation: (revision: number) => setValue(previous =>
    previous.revision === revision ? {...previous, invitation: ""} : previous) };
}
