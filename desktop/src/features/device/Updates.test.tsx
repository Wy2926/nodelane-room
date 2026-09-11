import { beforeEach, expect, test, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Updates } from "./Updates";
import { rpc } from "../../native/api";
import { setLanguage } from "../../i18n";

vi.mock("../../native/api", () => ({rpc: vi.fn(), failure: (e: unknown) => e}));
beforeEach(() => { vi.clearAllMocks(); setLanguage("zh-CN"); });

test("forced update is visible during download and installation is available only after verification", async () => {
  vi.mocked(rpc).mockResolvedValue({state: "downloading", required: true, downloaded: 50, release: {version: "0.3.1", size: 100, notes: "修复联机"}});
  render(<Updates />);
  expect(await screen.findByText("当前版本已停止联机授权，请完成更新后继续使用")).toBeTruthy();
  expect(screen.getByRole("progressbar").getAttribute("value")).toBe("50");
  expect(screen.queryByRole("button", {name: "立即更新"})).toBeNull();
  expect((screen.getByRole("button", {name: "检查更新"}) as HTMLButtonElement).disabled).toBe(true);
});

test("install requires explicit confirmation and only invokes the fixed local update action", async () => {
  vi.mocked(rpc).mockResolvedValue({state: "ready", required: false, downloaded: 100, release: {version: "0.3.1", size: 100}});
  render(<Updates />);
  await userEvent.click(await screen.findByRole("button", {name: "立即更新"}));
  expect(rpc).not.toHaveBeenCalledWith({action: "update-install"});
  await userEvent.click(screen.getByRole("button", {name: "确认安装并重启"}));
  await waitFor(() => expect(rpc).toHaveBeenCalledWith({action: "update-install"}));
});
