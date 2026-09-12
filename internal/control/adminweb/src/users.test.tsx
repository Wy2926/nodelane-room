// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Users } from "./users";
import type { API } from "./types";

afterEach(cleanup);

it("requires a reason before restricting room creation for the selected user", async () => {
  const account = {
    id: "user-a",
    name: "访客甲",
    kind: "guest",
    state: "active",
    created_at: new Date().toISOString(),
  };
  const api = vi.fn(async (path: string) =>
    path.includes("?")
      ? { users: [account], next: "" }
      : { user: account, devices: [], rooms: [], sessions: [] },
  );
  render(<Users api={api as API} />);
  await userEvent.click(await screen.findByRole("button", { name: "查看" }));
  await userEvent.click(
    await screen.findByRole("button", { name: "限制创建房间" }),
  );
  expect(await screen.findByText("请先填写操作原因")).toBeTruthy();
  expect(api.mock.calls.some(([path]) => path.endsWith("/actions"))).toBe(
    false,
  );
  await userEvent.type(screen.getByLabelText("操作原因"), "滥用联机");
  await userEvent.click(screen.getByRole("button", { name: "限制创建房间" }));
  await waitFor(() =>
    expect(api).toHaveBeenCalledWith("/users/user-a/actions", {
      action: "disable",
      device_id: undefined,
      reason: "滥用联机",
    }),
  );
});
