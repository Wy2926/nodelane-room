import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { useService } from "./use-service";
import { clientVersion, rpc } from "./api";
import type { Status } from "../shared/model";

vi.mock("./api", async (original) => ({
  ...(await original<typeof import("./api")>()), rpc: vi.fn(), notifyState: vi.fn(),
}));
beforeEach(() => vi.clearAllMocks());
afterEach(() => vi.useRealTimers());
const status = { version: clientVersion, protocol_version: 1, engine: "stopped", control: "unconfigured" } as Status;

test("refresh waits for the active request and schedules one follow-up", async () => {
  vi.useFakeTimers();
  let complete!: (value: Status) => void;
  vi.mocked(rpc).mockImplementationOnce(() => new Promise((resolve) => { complete = resolve as typeof complete; }));
  vi.mocked(rpc).mockResolvedValue(status);
  const view = renderHook(useService);
  act(() => { view.result.current.refresh(); view.result.current.refresh(); });
  expect(rpc).toHaveBeenCalledTimes(1);
  await act(async () => { complete(status); });
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
  expect(rpc).toHaveBeenCalledTimes(2);
  view.unmount();
  await act(async () => { await vi.advanceTimersByTimeAsync(20000); });
  expect(rpc).toHaveBeenCalledTimes(2);
});

test.each([
  [{ ...status, protocol_version: 2 }, "incompatible"],
  [{ ...status, version: "0.0.1" }, "version_mismatch"],
] as const)("rejects mismatched service metadata and recovers after replacement", async (remote, code) => {
  vi.mocked(rpc).mockResolvedValueOnce(remote);
  const view = renderHook(useService);
  await waitFor(() => expect(view.result.current.error?.code).toBe(code));
  expect(view.result.current.status).toBeUndefined();
  vi.mocked(rpc).mockResolvedValue(status);
  act(() => view.result.current.refresh());
  await waitFor(() => expect(view.result.current.status?.version).toBe(clientVersion));
  expect(view.result.current.error).toBeUndefined();
});
