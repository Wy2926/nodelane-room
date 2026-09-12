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
beforeEach(() => vi.resetAllMocks());

test.each([false, true])("a waiting action remains busy, rejects duplicate clicks, and ends after settlement (failed: %s)", async (failed) => {
  let complete!: (value: unknown) => void;
  let reject!: (reason: unknown) => void;
  vi.mocked(rpc).mockImplementationOnce(() => new Promise((resolve, fail) => {
    complete = resolve;
    reject = fail;
  }));
  const view = renderHook(() => useActions(vi.fn(), "service-a"));
  let pending!: Promise<boolean>;
  act(() => { pending = view.result.current.perform("Joining", { action: "join" }); });
  expect(view.result.current.busy).toBe("Joining");
  await act(async () => {
    expect(await view.result.current.perform("Joining", { action: "join" })).toBe(false);
  });
  expect(rpc).toHaveBeenCalledOnce();
  await act(async () => {
    if (failed) reject({ code: "room_full" });
    else complete({ room: { id: "joined" } });
    expect(await pending).toBe(!failed);
  });
  expect(view.result.current.busy).toBe("");
  expect(view.result.current.error?.code).toBe(failed ? "room_full" : undefined);
});

test("a late pause response cannot hide a new service's pause progress", async () => {
  let completeOld!: (value: unknown) => void;
  let rejectNew!: (reason: unknown) => void;
  vi.mocked(rpc)
    .mockImplementationOnce(() => new Promise((resolve) => { completeOld = resolve; }))
    .mockImplementationOnce(() => new Promise((_, reject) => { rejectNew = reject; }));
  const view = renderHook(({ instance }) => useActions(vi.fn(), instance), {
    initialProps: { instance: "old" },
  });
  let old!: Promise<boolean>;
  let next!: Promise<boolean>;
  act(() => { old = view.result.current.perform("Pausing", { action: "network-stop" }); });
  view.rerender({ instance: "new" });
  act(() => { next = view.result.current.perform("Pausing", { action: "network-stop" }); });
  await act(async () => {
    completeOld({});
    await old;
  });
  expect(view.result.current.pausing).toBe(true);
  await act(async () => {
    rejectNew({ code: "local_service_unavailable" });
    await next;
  });
  expect(view.result.current.pausing).toBe(false);
  expect(view.result.current.error?.code).toBe("local_service_unavailable");
});

test("uncertain writes block new intents while pause and original receipt remain available", async () => {
  vi.mocked(rpc)
    .mockRejectedValueOnce({
      code: "future_business_code",
      message: "private internal details",
      request_id: "control-request",
    })
    .mockResolvedValueOnce({});
  const view = renderHook(() => useActions(vi.fn(), "service-a"));
  await act(async () => {
    await view.result.current.perform("create", {
      action: "create",
      body: { name: "room" },
    });
  });
  const original = vi.mocked(rpc).mock.calls[0][0].command_id;
  vi.mocked(rpc).mockResolvedValueOnce({
    id: original,
    state: "rejected",
    result: { code: "room_full" },
  });
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

test.each([false, true])(
  "takeover resumes join after confirmation (receipt: %s)",
  async (receipt) => {
    vi.mocked(rpc).mockRejectedValueOnce({
      code: "account_in_use",
      details: {
        room_id: "occupied",
        device_id: "old-device",
        actual_revision: 7,
      },
    });
    if (receipt)
      vi.mocked(rpc).mockRejectedValueOnce({ code: "local_rpc_timeout" });
    else vi.mocked(rpc).mockResolvedValueOnce({});
    vi.mocked(rpc).mockResolvedValue({ room: { id: "joined" } });
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
    if (receipt) {
      vi.mocked(rpc).mockResolvedValueOnce({
        id: view.result.current.pending,
        state: "succeeded",
        result: { data: {} },
      });
    }
    if (receipt)
      await act(async () => {
        await view.result.current.checkOperation();
      });
    const requests = vi
      .mocked(rpc)
      .mock.calls.map(([request]) => request)
      .filter((request) => request.action !== "get-operation");
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
  },
);

test.each([false, true])(
  "old receipt queries cannot unlock a new query (review: %s)",
  async (review) => {
    let rejectOld!: (error: unknown) => void;
    let resolveNew!: (value: unknown) => void;
    vi.mocked(rpc)
      .mockImplementationOnce(
        () =>
          new Promise((_, reject) => {
            rejectOld = reject;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveNew = resolve;
          }),
      );
    const view = renderHook(
      ({ instance }) =>
        useActions(vi.fn(), instance, {
          operations: [
            {
              id: "pending",
              state: review ? "unresolved" : "pending",
              known_commit: false,
              deadline: "2026-09-12T00:00:00Z",
            },
          ],
          unavailable: false,
          roomBlocked: false,
        }),
      { initialProps: { instance: "old" } },
    );
    let old!: Promise<void>;
    act(() => {
      old = review
        ? view.result.current.reviewPending()
        : view.result.current.checkOperation();
    });
    view.rerender({ instance: "new" });
    let next!: Promise<void>;
    act(() => {
      next = view.result.current.checkOperation();
    });
    await act(async () => {
      rejectOld({ code: "local_control_timeout" });
      await old;
    });
    expect(view.result.current.checking).toBe(true);
    expect(view.result.current.error).toBeUndefined();
    await act(async () => {
      resolveNew({
        id: "pending",
        state: "rejected",
        result: { code: "room_full" },
      });
      await next;
    });
    expect(view.result.current.pending).toBeUndefined();
    expect(view.result.current.error?.code).toBe("room_full");
  },
);

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
