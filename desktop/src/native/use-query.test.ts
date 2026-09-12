import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { rpc } from "./api";
import { useQuery } from "./use-query";

vi.mock("./api", async (original) => ({
  ...(await original<typeof import("./api")>()),
  rpc: vi.fn(),
}));
beforeEach(() => {
  vi.resetAllMocks();
  vi.useFakeTimers();
});
afterEach(() => vi.useRealTimers());

test("background polling retains the last response without showing initial loading again", async () => {
  let complete!: (value: unknown) => void;
  vi.mocked(rpc).mockImplementation(() => new Promise((resolve) => {
    complete = resolve;
  }));
  const view = renderHook(() => useQuery({ action: "rooms" }, 15000));
  expect(view.result.current.loading).toBe(true);
  await act(async () => complete({ rooms: [{ id: "first" }] }));
  expect(view.result.current.loading).toBe(false);
  await act(async () => vi.advanceTimersByTime(15000));
  expect(rpc).toHaveBeenCalledTimes(2);
  expect(view.result.current.loading).toBe(false);
  expect(view.result.current.data).toEqual({ rooms: [{ id: "first" }] });
  await act(async () => complete({ rooms: [{ id: "updated" }] }));
  expect(view.result.current.data).toEqual({ rooms: [{ id: "updated" }] });
});

test.each([false, true])("manual reload shows waiting and keeps the last response until settlement (failed: %s)", async (failed) => {
  const original = { rooms: [{ id: "first" }] };
  const refreshed = { rooms: [{ id: "updated" }] };
  let complete!: (value: unknown) => void;
  let reject!: (reason: unknown) => void;
  vi.mocked(rpc)
    .mockResolvedValueOnce(original)
    .mockImplementationOnce(() => new Promise((resolve, fail) => {
      complete = resolve;
      reject = fail;
    }));
  const view = renderHook(({ reload }) => useQuery({ action: "rooms" }, 0, { reload }), {
    initialProps: { reload: 0 },
  });
  await act(async () => {});
  expect(view.result.current.loading).toBe(false);
  const updatedAt = view.result.current.updatedAt;
  act(() => vi.advanceTimersByTime(1000));
  view.rerender({ reload: 1 });
  expect(view.result.current.loading).toBe(true);
  expect(view.result.current.data).toEqual(original);
  expect(view.result.current.updatedAt).toBe(updatedAt);
  await act(async () => {
    if (failed) reject({ code: "local_control_timeout" });
    else complete(refreshed);
  });
  expect(view.result.current.loading).toBe(false);
  expect(view.result.current.data).toEqual(failed ? original : refreshed);
  expect(view.result.current.error?.code).toBe(failed ? "local_control_timeout" : undefined);
  expect(view.result.current.updatedAt).toBe(failed ? updatedAt : updatedAt + 1000);
});

test("failed initial reads finish loading and a replaced request ignores its old response", async () => {
  let completeOld!: (value: unknown) => void;
  let rejectNew!: (reason: unknown) => void;
  vi.mocked(rpc)
    .mockImplementationOnce(() => new Promise((resolve) => { completeOld = resolve; }))
    .mockImplementationOnce(() => new Promise((_, reject) => { rejectNew = reject; }));
  const view = renderHook(({ room }) => useQuery({ action: "manage", room }, 0), {
    initialProps: { room: "first" },
  });
  view.rerender({ room: "second" });
  await act(async () => completeOld({ room: { id: "first" } }));
  expect(view.result.current.loading).toBe(true);
  expect(view.result.current.data).toBeUndefined();
  await act(async () => rejectNew({ code: "local_control_timeout" }));
  expect(view.result.current.loading).toBe(false);
  expect(view.result.current.error?.code).toBe("local_control_timeout");
});
