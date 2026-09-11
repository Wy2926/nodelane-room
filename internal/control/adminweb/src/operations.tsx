import { useEffect, useState } from "react";
import type { API } from "./types";
import { forgetOperation, pendingOperations } from "./api";

export function PendingOperations({
  api,
  refresh,
}: {
  api: API;
  refresh: () => Promise<void>;
}) {
  const [pending, setPending] = useState(pendingOperations);
  const [message, setMessage] = useState("");
  useEffect(() => {
    const timer = setInterval(() => setPending(pendingOperations()), 2000);
    return () => clearInterval(timer);
  }, []);
  async function check(id: string) {
    try {
      const op = await api<{ state: string; result?: { code: string } }>(
        `/operations/${id}`,
      );
      if (op.state === "succeeded" || op.state === "rejected") {
        forgetOperation(id);
        setPending(pendingOperations());
        setMessage(
          op.state === "succeeded"
            ? "原操作已确认完成，已刷新当前状态。"
            : "原操作已拒绝，请核对当前状态后重新确认。",
        );
        await refresh();
      }
    } catch (e) {
      setMessage((e as Error).message);
    }
  }
  if (!pending.length && !message) return null;
  return (
    <section aria-label="未决操作">
      <p role="status">
        {message || "有操作结果待确认；查询不会再次执行操作。"}
      </p>
      {pending.map((op) => (
        <p key={op.id}>
          <code>{op.id}</code>{" "}
          <button onClick={() => void check(op.id)}>查询原操作</button>
          {Date.parse(op.deadline) <= Date.now() && (
            <button
              onClick={() => {
                void refresh().then(() => {
                  forgetOperation(op.id);
                  setPending(pendingOperations());
                  setMessage(
                    "恢复窗口已结束，请核对页面当前状态后再提交新操作。",
                  );
                });
              }}
            >
              核对当前状态
            </button>
          )}
        </p>
      ))}
    </section>
  );
}
