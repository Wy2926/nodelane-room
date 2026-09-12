import { useEffect, useRef, useState } from "react";
import { failure, rpc } from "../native/api";
import type { Failure, Operation, Status } from "../shared/model";

type Pending = {
  id: string;
  unresolved?: boolean;
  reviewed?: boolean;
  success?: (value: unknown) => void;
  followup?: () => Promise<unknown>;
};

// The service owns durable receipts; callbacks belong only to this UI instance.
export function useOperations(options: {
  instance?: string;
  operations?: Operation[];
  unavailable?: boolean;
  busy: { current: boolean };
  refresh: () => void;
  setError: (error: Failure | undefined) => void;
  closeDialog: () => void;
}) {
  const latest = useRef(options);
  latest.current = options;
  const current = useRef<Pending>(undefined);
  const [pending, setPending] = useState<Pending>();
  const [checking, setChecking] = useState(false);
  const query = useRef<object>(undefined);
  const acknowledged = useRef(new Set<string>());
  function update(value?: Pending) {
    current.current = value;
    setPending(value);
  }
  useEffect(() => {
    // Retain the original ID for recovery, but never replay a former UI intent.
    if (current.current) update({ id: current.current.id });
    query.current = undefined;
    setChecking(false);
    return () => {
      query.current = undefined;
    };
  }, [options.instance]);
  useEffect(() => {
    if (!options.operations) return;
    const ids = new Set(options.operations.map((op) => op.id));
    for (const id of acknowledged.current)
      if (!ids.has(id)) acknowledged.current.delete(id);
    const operation = options.operations.find(
      (op) => !acknowledged.current.has(op.id),
    );
    if (!current.current && operation)
      update({
        id: operation.id,
        unresolved: operation.state === "unresolved",
      });
  }, [options.operations]);
  useEffect(() => {
    if (!pending || pending.unresolved || options.unavailable) return;
    const timer = setInterval(() => void checkOperation(), 3000);
    return () => clearInterval(timer);
  }, [pending?.id, pending?.unresolved, options.unavailable, options.instance]);

  async function queryPending(review: boolean, id = current.current?.id) {
    const item = current.current;
    const { instance, busy, unavailable } = latest.current;
    if (!item || item.id !== id || query.current || busy.current || unavailable)
      return;
    const ticket = {};
    query.current = ticket;
    setChecking(true);
    const valid = () =>
      query.current === ticket &&
      latest.current.instance === instance &&
      current.current === item;
    try {
      if (review) {
        const status = await rpc<Status>({ action: "status" });
        if (!valid()) return;
        if (status.control !== "online")
          throw { code: "local_control_unreachable" };
        latest.current.closeDialog();
        update({ ...item, reviewed: true });
      } else {
        const operation = await rpc<Operation>({
          action: "get-operation",
          target: id,
        });
        if (!valid()) return;
        if (operation.id !== id) throw { code: "local_ipc_response_invalid" };
        switch (operation.state) {
          case "succeeded":
          case "rejected":
            acknowledged.current.add(item.id);
            update();
            latest.current.setError(
              operation.state === "rejected"
                ? failure(operation.result)
                : undefined,
            );
            if (operation.state === "succeeded") {
              if (operation.result?.data != null)
                item.success?.(operation.result.data);
              else latest.current.closeDialog();
              await item.followup?.();
            }
            break;
          case "unresolved":
            update({ id: item.id, unresolved: true });
            latest.current.setError(failure({ code: "operation_expired" }));
            break;
          default:
            update({ ...item, unresolved: false, reviewed: false });
        }
      }
      latest.current.refresh();
    } catch (e) {
      if (valid()) {
        const issue = failure(e);
        latest.current.setError(issue);
        if (issue.code === "operation_expired")
          update({ id: item.id, unresolved: true });
      }
    } finally {
      if (query.current === ticket) {
        query.current = undefined;
        setChecking(false);
      }
    }
  }
  function checkOperation(id?: string) {
    return queryPending(false, id);
  }
  async function reviewPending() {
    const item = current.current;
    if (
      !item?.unresolved ||
      query.current ||
      latest.current.busy.current ||
      latest.current.unavailable
    )
      return;
    if (!item.reviewed) return queryPending(true);
    acknowledged.current.add(item.id);
    update();
    latest.current.setError(undefined);
    latest.current.refresh();
  }
  return {
    pending: pending?.id,
    unresolved: !!pending?.unresolved,
    reviewed: !!pending?.reviewed,
    checking,
    checkOperation,
    reviewPending,
    hasPending: () => !!current.current,
    track: (id: string, success?: Pending["success"]) =>
      update({ id, success }),
    followup: (next: Pending["followup"]) => {
      if (current.current) update({ ...current.current, followup: next });
    },
  };
}
