import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { invoke, isTauri } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { useInvitation } from "./use-invitation";

vi.mock("@tauri-apps/api/core", () => ({ invoke: vi.fn(), isTauri: vi.fn() }));
vi.mock("@tauri-apps/api/event", () => ({ listen: vi.fn() }));
const code = "0123456789abcdef0123456789abcdef";
const unlisten = vi.fn();
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(isTauri).mockReturnValue(true);
  vi.mocked(listen).mockResolvedValue(unlisten);
  vi.mocked(invoke).mockResolvedValue(null);
});
function ready() {
  const [event, callback] = vi.mocked(listen).mock.calls[0];
  callback({ event, id: 1, payload: null });
}

test("registers the warm-launch listener before taking a cold-launch invitation", async () => {
  let registered!: (off: () => void) => void;
  vi.mocked(listen).mockImplementationOnce(() => new Promise((resolve) => {
    registered = resolve;
  }));
  vi.mocked(invoke).mockResolvedValue(code);
  const view = renderHook(useInvitation);
  expect(listen).toHaveBeenCalledWith("invitation-ready", expect.any(Function));
  expect(invoke).not.toHaveBeenCalled();
  await act(async () => registered(unlisten));
  await waitFor(() => expect(view.result.current.invitation).toBe(code));
  expect(invoke).toHaveBeenCalledExactlyOnceWith("take_invitation");
  expect(view.result.current.revision).toBe(1);
});

test("warm launches update the invitation and reopening the same link advances its revision", async () => {
  const view = renderHook(useInvitation);
  await waitFor(() => expect(invoke).toHaveBeenCalledOnce());
  vi.mocked(invoke).mockResolvedValue(code);
  await act(async () => ready());
  expect(view.result.current.invitation).toBe(code);
  expect(view.result.current.revision).toBe(1);
  act(() => view.result.current.clearInvitation(view.result.current.revision));
  expect(view.result.current.invitation).toBe("");
  await act(async () => ready());
  expect(view.result.current.invitation).toBe(code);
  expect(view.result.current.revision).toBe(2);
  vi.mocked(invoke).mockResolvedValue("untrusted-link");
  await act(async () => ready());
  expect(view.result.current.invitation).toBe(code);
  expect(view.result.current.revision).toBe(2);
});

test("a warm event during a cold read waits, then drains the newest invitation in order", async () => {
  const newerCode = "fedcba9876543210fedcba9876543210";
  let cold!: (value: unknown) => void;
  let warm!: (value: unknown) => void;
  vi.mocked(invoke)
    .mockImplementationOnce(() => new Promise((resolve) => { cold = resolve; }))
    .mockImplementationOnce(() => new Promise((resolve) => { warm = resolve; }));
  const view = renderHook(useInvitation);
  await waitFor(() => expect(invoke).toHaveBeenCalledOnce());
  act(() => { ready(); ready(); });
  expect(invoke).toHaveBeenCalledOnce();
  await act(async () => cold(code));
  expect(invoke).toHaveBeenCalledTimes(2);
  expect(view.result.current.invitation).toBe(code);
  await act(async () => warm(newerCode));
  expect(view.result.current.invitation).toBe(newerCode);
  expect(view.result.current.revision).toBe(2);
  expect(invoke).toHaveBeenCalledTimes(2);
});

test("completing an older join cannot clear a newer invitation revision", async () => {
  vi.mocked(invoke).mockResolvedValue(code);
  const view = renderHook(useInvitation);
  await waitFor(() => expect(view.result.current.revision).toBe(1));
  const submittedRevision = view.result.current.revision;
  const newerCode = "fedcba9876543210fedcba9876543210";
  vi.mocked(invoke).mockResolvedValue(newerCode);
  await act(async () => ready());
  expect(view.result.current.revision).toBe(2);
  act(() => view.result.current.clearInvitation(submittedRevision));
  expect(view.result.current.invitation).toBe(newerCode);
  act(() => view.result.current.clearInvitation(view.result.current.revision));
  expect(view.result.current.invitation).toBe("");
});

test("unmounting before listener registration removes it without consuming a pending invitation", async () => {
  let registered!: (off: () => void) => void;
  vi.mocked(listen).mockImplementationOnce(() => new Promise((resolve) => {
    registered = resolve;
  }));
  const view = renderHook(useInvitation);
  view.unmount();
  await act(async () => registered(unlisten));
  expect(unlisten).toHaveBeenCalledOnce();
  expect(invoke).not.toHaveBeenCalled();
});

test("a queued ready event after unmount does not consume the next window's invitation", async () => {
  const view = renderHook(useInvitation);
  await waitFor(() => expect(invoke).toHaveBeenCalledOnce());
  view.unmount();
  expect(unlisten).toHaveBeenCalledOnce();
  await act(async () => ready());
  expect(invoke).toHaveBeenCalledOnce();
});
