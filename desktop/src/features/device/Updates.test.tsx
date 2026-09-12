import { beforeEach, expect, test, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Updates, UpdatePrompt } from "./Updates";
import { rpc, clientVersion } from "../../native/api";
import { setLanguage } from "../../i18n";

vi.mock("../../native/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../native/api")>()),
  rpc: vi.fn(),
}));
beforeEach(() => {
  vi.clearAllMocks();
  localStorage.removeItem("nlroom.skipped-update");
  setLanguage("zh-CN");
});

test("forced update is visible during download and installation is available only after verification", async () => {
  vi.mocked(rpc).mockResolvedValue({
    state: "downloading",
    required: true,
    downloaded: 50,
    release: { version: "0.3.1", size: 100, notes: "修复联机" },
  });
  render(<Updates />);
  expect(
    await screen.findByText("当前版本已停止联机授权，请完成更新后继续使用"),
  ).toBeTruthy();
  expect(screen.getByRole("progressbar").getAttribute("value")).toBe("50");
  expect(screen.queryByRole("button", { name: "立即更新" })).toBeNull();
  expect(
    (screen.getByRole("button", { name: "检查更新" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
});

test("a verified package starts installation with one click", async () => {
  vi.mocked(rpc).mockResolvedValue({
    state: "ready",
    required: false,
    downloaded: 100,
    release: { id: "release1", version: "0.3.1", size: 100 },
  });
  render(<UpdatePrompt />);
  await userEvent.click(
    await screen.findByRole("button", { name: "立即更新" }),
  );
  await waitFor(() =>
    expect(rpc).toHaveBeenCalledWith({ action: "update-install" }),
  );
});

test("ready update has one primary action and release notes are collapsed", async () => {
  vi.mocked(rpc).mockResolvedValue({
    state: "ready",
    required: false,
    downloaded: 100,
    release: { version: "0.3.1", size: 100, notes: "改进房间连接" },
  });
  render(<Updates />);
  expect(await screen.findByRole("button", { name: "立即更新" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "检查更新" })).toBeNull();
  const details = screen.getByText("改进房间连接").closest("details")!;
  expect(details.open).toBe(false);
  await userEvent.click(screen.getByText("更新内容"));
  expect(details.open).toBe(true);
});

test("unavailable update service keeps the client version and explains the failure", async () => {
  vi.mocked(rpc).mockRejectedValue({ code: "local_service_unavailable" });
  render(<Updates />);
  expect(
    screen.getByRole("heading", { name: `v${clientVersion}` }),
  ).toBeTruthy();
  expect((await screen.findByRole("alert")).textContent).toContain(
    "本机服务不可用",
  );
  expect(screen.queryByRole("button", { name: "立即更新" })).toBeNull();
  expect(
    (screen.getByRole("button", { name: "检查更新" }) as HTMLButtonElement)
      .disabled,
  ).toBe(false);
});

test("startup offers three actions and persists a skipped version across mounts", async () => {
  vi.mocked(rpc).mockResolvedValue({
    state: "available",
    downloaded: 0,
    required: false,
    release: {
      id: "release1",
      version: "0.4.0",
      size: 1048576,
      notes: "New release",
    },
  });
  const view = render(<UpdatePrompt />);
  expect(await screen.findByRole("dialog")).toBeTruthy();
  expect(screen.getByRole("button", { name: "取消" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "立即更新" })).toBeTruthy();
  await userEvent.click(screen.getByRole("button", { name: "跳过此版本" }));
  expect(localStorage.getItem("nlroom.skipped-update")).toBe("0.4.0");
  view.unmount();
  render(<UpdatePrompt />);
  await waitFor(() =>
    expect(rpc).toHaveBeenCalledWith({ action: "update-status" }),
  );
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(rpc).not.toHaveBeenCalledWith({
    action: "update-download",
    target: "release1",
  });
});

test("download displays actual progress and speed then installs once without confirmation", async () => {
  let state = "available";
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "update-download") state = "downloading";
    return {
      state,
      downloaded: state === "available" ? 0 : 524288,
      required: false,
      release: { id: "release1", version: "0.4.0", size: 1048576 },
    } as never;
  });
  render(<UpdatePrompt />);
  await userEvent.click(
    await screen.findByRole("button", { name: "立即更新" }),
  );
  await waitFor(
    () =>
      expect(screen.getByRole("progressbar").getAttribute("value")).toBe("50"),
    { timeout: 2500 },
  );
  expect(screen.getByText(/MB\/s/)).toBeTruthy();
  state = "ready";
  await waitFor(
    () => expect(rpc).toHaveBeenCalledWith({ action: "update-install" }),
    { timeout: 2500 },
  );
  expect(
    vi.mocked(rpc).mock.calls.filter(([r]) => r.action === "update-install"),
  ).toHaveLength(1);
});

test("cancelling a download prevents installation even when a late ready status arrives", async () => {
  let state = "available";
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "update-download") state = "downloading";
    if (request.action === "update-cancel") state = "ready";
    return {
      state,
      downloaded: 0,
      required: false,
      release: { id: "release1", version: "0.4.0", size: 100 },
    } as never;
  });
  render(<UpdatePrompt />);
  await userEvent.click(
    await screen.findByRole("button", { name: "立即更新" }),
  );
  await userEvent.click(screen.getByRole("button", { name: "取消" }));
  await new Promise((resolve) => setTimeout(resolve, 1200));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(rpc).toHaveBeenCalledWith({ action: "update-cancel" });
  expect(rpc).not.toHaveBeenCalledWith({ action: "update-install" });
});

test("required versions cannot be skipped even if previously skipped", async () => {
  localStorage.setItem("nlroom.skipped-update", "0.4.0");
  vi.mocked(rpc).mockResolvedValue({
    state: "available",
    downloaded: 0,
    required: true,
    release: { id: "release1", version: "0.4.0", size: 100 },
  });
  render(<UpdatePrompt />);
  expect(
    (
      (await screen.findByRole("button", {
        name: "跳过此版本",
      })) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
});
