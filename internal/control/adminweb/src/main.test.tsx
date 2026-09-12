// @vitest-environment jsdom
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { App } from "./main";

const { api } = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock("./api", async (original) => ({
  ...(await original<typeof import("./api")>()),
  makeAPI: () => api,
  watchAdmin: () => ({ close: vi.fn() }),
}));

beforeEach(() => {
  api.mockReset();
  api.mockImplementation(async (path: string) => {
    if (path === "/setup") return { initialized: true, configured: true };
    if (path === "/session") return { username: "admin", csrf: "test-only" };
    if (path === "/snapshot")
      return {
        nodes: [],
        rooms: [],
        games: [],
        events: [],
        operations: [],
        version: "0.3.0",
        server_time: new Date().toISOString(),
        public_url: "https://room.example",
        ca_expires_at: "2099-01-01T00:00:00Z",
      };
    if (path === "/telemetry")
      return {
        server_time: new Date().toISOString(),
        series: [],
        geoip: false,
      };
    if (path.startsWith("/users?")) return { users: [], next: "" };
    if (path === "/oidc")
      return {
        revision: 1,
        issuer: "https://id.example/oidc",
        client_id: "room",
        enabled: true,
      };
    if (path.startsWith("/updates?"))
      return {
        sources: [],
        releases: [],
        policies: [],
        devices: [],
        versions: [],
        attempts: [],
        repository_revision: 0,
      };
    return {};
  });
});
afterEach(() => {
  cleanup();
  window.history.replaceState(null, "", "/");
});

it("opens a bookmarked setting and restores the page through browser history", async () => {
  window.history.replaceState(null, "", "/#oidc");
  render(<App />);
  expect(
    await screen.findByRole("heading", { level: 1, name: "账号登录" }),
  ).toBeTruthy();
  expect(await screen.findByLabelText("Issuer")).toBeTruthy();
  expect(screen.queryByText("内存窗口 · 60 秒")).toBeNull();
  const nav = screen.getByRole("navigation", { name: "主导航" });
  expect(
    within(nav)
      .getByRole("link", { name: "账号登录" })
      .getAttribute("aria-current"),
  ).toBe("page");
  await userEvent.click(within(nav).getByRole("link", { name: "用户管理" }));
  expect(
    await screen.findByRole("heading", { level: 1, name: "用户管理" }),
  ).toBeTruthy();
  expect(screen.queryByLabelText("Issuer")).toBeNull();
  window.history.back();
  expect(
    await screen.findByRole("heading", { level: 1, name: "账号登录" }),
  ).toBeTruthy();
  expect(await screen.findByLabelText("Issuer")).toBeTruthy();
});

it("separates storage settings, release operations, device reports and deployment information", async () => {
  window.history.replaceState(null, "", "/#sources");
  render(<App />);
  expect(
    await screen.findByRole("button", { name: "添加存储源" }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: "创建版本" })).toBeNull();
  const nav = screen.getByRole("navigation", { name: "主导航" });
  await userEvent.click(
    within(nav).getByRole("link", { name: "版本与安装包" }),
  );
  expect(await screen.findByRole("button", { name: "创建版本" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "添加存储源" })).toBeNull();
  await userEvent.click(within(nav).getByRole("link", { name: "设备版本" }));
  expect(
    await screen.findByRole("heading", { name: "设备版本与更新结果" }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: "创建版本" })).toBeNull();
  await userEvent.click(within(nav).getByRole("link", { name: "部署信息" }));
  expect(
    await screen.findByRole("heading", { level: 1, name: "部署信息" }),
  ).toBeTruthy();
  expect(screen.queryByLabelText("当前密码")).toBeNull();
});

it("returns to login after the dedicated administrator password form succeeds", async () => {
  window.history.replaceState(null, "", "/#password");
  render(<App />);
  await userEvent.type(
    await screen.findByLabelText("当前密码"),
    "current-test-password",
  );
  await userEvent.type(
    screen.getByLabelText("新密码（12–128 字节）"),
    "new-test-password",
  );
  await userEvent.click(screen.getByRole("button", { name: "保存新密码" }));
  await waitFor(() =>
    expect(api).toHaveBeenCalledWith("/password", {
      current: "current-test-password",
      password: "new-test-password",
    }),
  );
  expect(await screen.findByText("密码已修改，请重新登录。")).toBeTruthy();
  expect(screen.queryByRole("navigation", { name: "主导航" })).toBeNull();
});
