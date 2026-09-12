import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { useAccount } from "./use-account";
import { rpc } from "../../native/api";
import type { Actions } from "../../app/use-actions";
import type { Status } from "../../shared/model";

vi.mock("../../native/api", async (original) => ({
  ...(await original<typeof import("../../native/api")>()),
  rpc: vi.fn(),
}));
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(rpc).mockImplementation(async ({ action }) => {
    if (action === "capabilities")
      return { ready: true, oidc_enabled: true } as never;
    if (action === "account-devices")
      return [{ device_id: "device", name: "PC" }] as never;
    if (action === "account-status") return {} as never;
    return { state: "none" } as never;
  });
});
const status = {
  service_instance_id: "old",
  user: { id: "account", kind: "guest" },
} as Status;
const actions = { perform: vi.fn() } as unknown as Actions;

test("binding a guest loads devices even when the account ID stays the same", async () => {
  const view = renderHook(({ value }) => useAccount(value, actions, false), {
    initialProps: { value: status },
  });
  view.rerender({
    value: { ...status, user: { ...status.user!, kind: "registered" } },
  });
  await waitFor(() => expect(view.result.current.devices).toHaveLength(1));
  vi.mocked(rpc).mockRejectedValue({ code: "local_control_unreachable" });
  view.rerender({
    value: {
      ...status,
      user: { ...status.user!, id: "other", kind: "registered" },
    },
  });
  await waitFor(() => expect(view.result.current.message).not.toBe(""));
  expect(view.result.current.devices).toEqual([]);
});

test("login completion uses current actions and ignores a replaced service", async () => {
  let complete!: (value: unknown) => void;
  vi.mocked(rpc).mockImplementation(async ({ action }) => {
    if (action === "account-poll")
      return new Promise((resolve) => {
        complete = resolve;
      });
    return {} as never;
  });
  const updated = { perform: vi.fn() } as unknown as Actions;
  const view = renderHook(
    ({ value, handler }) => useAccount(value, handler, false),
    {
      initialProps: { value: status, handler: actions },
    },
  );
  view.rerender({ value: status, handler: updated });
  await act(async () => {
    complete({ state: "ready" });
  });
  expect(updated.perform).toHaveBeenCalledOnce();
  expect(actions.perform).not.toHaveBeenCalled();
  view.rerender({
    value: { ...status, service_instance_id: "next" },
    handler: updated,
  });
  const obsolete = complete;
  view.rerender({
    value: { ...status, service_instance_id: "final" },
    handler: updated,
  });
  await act(async () => {
    obsolete({ state: "ready" });
  });
  expect(updated.perform).toHaveBeenCalledOnce();
});
