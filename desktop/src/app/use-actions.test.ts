import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { useActions } from "./use-actions";
import { failure, rpc } from "../native/api";

vi.mock("../native/api", async (original) => ({
  ...(await original<typeof import("../native/api")>()),
  rpc: vi.fn(),
}));
vi.mock("@tauri-apps/api/core", () => ({
  isTauri: () => false,
  invoke: vi.fn(),
}));
beforeEach(() => vi.clearAllMocks());

test("uncertain writes block new intents while pause and original receipt remain available", async () => {
  vi.mocked(rpc)
    .mockRejectedValueOnce({
      code: "future_business_code",
      message: "private internal details",
      request_id: "control-request",
    })
    .mockResolvedValueOnce({})
    .mockResolvedValueOnce({
      id: "original",
      state: "rejected",
      result: { code: "room_full" },
    });
  const view = renderHook(() => useActions(vi.fn(), "service-a"));
  await act(async () => {
    await view.result.current.perform("create", {
      action: "create",
      body: { name: "room" },
    });
  });
  const original = vi.mocked(rpc).mock.calls[0][0].command_id;
  expect(view.result.current.pending).toBe(original);
  expect(view.result.current.error?.error).not.toContain(
    "private internal details",
  );
  await act(async () => {
    expect(
      await view.result.current.perform("create", { action: "create" }),
    ).toBe(false);
    await view.result.current.perform("pause", { action: "network-stop" });
  });
  expect(rpc).toHaveBeenCalledTimes(2);
  await act(async () => {
    await view.result.current.checkOperation(original);
  });
  expect(rpc).toHaveBeenLastCalledWith({
    action: "get-operation",
    target: original,
  });
  expect(view.result.current.pending).toBeUndefined();
  expect(view.result.current.error?.code).toBe("room_full");
});

test("confirmed takeover continues the original join with a new child operation", async () => {
  vi.mocked(rpc)
    .mockRejectedValueOnce({
      code: "account_in_use",
      details: {
        room_id: "occupied",
        device_id: "old-device",
        actual_revision: 7,
      },
    })
    .mockResolvedValueOnce({})
    .mockResolvedValueOnce({ room: { id: "joined" } });
  const success = vi.fn();
  const view = renderHook(() => useActions(vi.fn(), "service-a"));
  await act(async () => {
    await view.result.current.perform(
      "join",
      { action: "join", body: { code: "memory-only" } },
      success,
    );
  });
  expect(view.result.current.takeover?.device).toBe("old-device");
  await act(async () => {
    await view.result.current.takeOverAndContinue();
  });
  const requests = vi.mocked(rpc).mock.calls.map(([request]) => request);
  expect(requests.map((r) => r.action)).toEqual([
    "join",
    "account-takeover",
    "join",
  ]);
  expect(requests[1].body).toEqual({
    room_id: "occupied",
    device_id: "old-device",
    expected_revision: 7,
  });
  expect(requests[2].body).toEqual(requests[0].body);
  expect(new Set(requests.map((r) => r.command_id)).size).toBe(3);
  expect(success).toHaveBeenCalledOnce();
});

test("a response from the previous service instance cannot complete the new screen", async () => {
  let complete!: (value: unknown) => void;
  vi.mocked(rpc).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  const success = vi.fn();
  const view = renderHook(({ instance }) => useActions(vi.fn(), instance), {
    initialProps: { instance: "old" },
  });
  let operation!: Promise<boolean>;
  act(() => {
    operation = view.result.current.perform(
      "join",
      { action: "join" },
      success,
    );
  });
  view.rerender({ instance: "new" });
  await act(async () => {
    complete({ room: { id: "obsolete" } });
    await operation;
  });
  expect(success).not.toHaveBeenCalled();
  expect(view.result.current.pending).toBeUndefined();
  expect(
    failure({ code: "unrecognized", message: "secret", request_id: "safe-id" })
      .request_id,
  ).toBe("safe-id");
});
