// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Auth } from "./auth";
import type { API } from "./types";

afterEach(cleanup);
it("submits credentials and enters the authenticated app", async () => {
  const api = vi.fn().mockResolvedValue({ username: "admin", csrf: "csrf" }),
    signedIn = vi.fn();
  render(<Auth api={api as API} signedIn={signedIn} />);
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("管理员账号"), "admin");
  await user.type(screen.getByLabelText("密码"), "correct password");
  await user.click(screen.getByRole("button", { name: "登录管理台" }));
  expect(api).toHaveBeenCalledWith("/login", {
    username: "admin",
    password: "correct password",
  });
  expect(signedIn).toHaveBeenCalledWith({ username: "admin", csrf: "csrf" });
});
it("preserves the existing-control setup flow without creation fields", async () => {
  const api = vi
    .fn()
    .mockResolvedValue({ public_url: "https://room.example.com" });
  render(<Auth api={api as API} signedIn={() => {}} initiallySetup />);
  const user = userEvent.setup();
  await user.selectOptions(screen.getByLabelText("部署方式"), "connect");
  expect(screen.queryByLabelText("游戏地址池")).toBeNull();
  await user.type(screen.getByLabelText("管理员账号"), "admin");
  await user.type(screen.getByLabelText("密码"), "correct password");
  await user.type(screen.getByLabelText("初始化码"), "a".repeat(64));
  await user.type(
    screen.getByLabelText("PostgreSQL 连接串"),
    "postgres://test:test@db/test",
  );
  await user.click(screen.getByRole("button", { name: "保存并启动" }));
  expect(api).toHaveBeenCalledWith(
    "/setup",
    expect.objectContaining({ mode: "connect", code: "a".repeat(64) }),
  );
  expect(screen.getByRole("alert").textContent).toContain("配置已保存");
});
