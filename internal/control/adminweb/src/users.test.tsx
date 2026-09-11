// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { OIDCSettings, Users } from "./users";
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
  await userEvent.click(
    screen.getByRole("button", { name: "限制创建房间" }),
  );
  await waitFor(() =>
    expect(api).toHaveBeenCalledWith("/users/user-a/actions", {
      action: "disable",
      device_id: undefined,
      reason: "滥用联机",
    }),
  );
});

it("saves OIDC configuration and clears the secret field", async () => {
  let config = {
    revision: 0,
    issuer: "https://id.example",
    client_id: "room",
    enabled: false,
  };
  const api = vi.fn(
    async (
      _path: string,
      body?: typeof config & { client_secret?: string },
      method?: string,
    ) => {
      if (method === "PUT" && body)
        config = {
          revision: body.revision,
          issuer: body.issuer,
          client_id: body.client_id,
          enabled: body.enabled,
        };
      return config;
    },
  );
  const app = render(
    <OIDCSettings api={api as API} publicURL="https://room.example/" />,
  );
  const secret = await screen.findByLabelText(
    "Client secret（相同客户端留空保留，不回显）",
  );
  expect(
    screen.getByText("https://room.example/v2/auth/oidc/callback"),
  ).toBeTruthy();
  await userEvent.clear(screen.getByLabelText("Issuer"));
  await userEvent.type(
    screen.getByLabelText("Issuer"),
    "https://auth.nodelane.net/oidc",
  );
  await userEvent.clear(screen.getByLabelText("Client ID"));
  await userEvent.type(screen.getByLabelText("Client ID"), "logto-app");
  await userEvent.type(secret, "test-only-secret");
  await userEvent.click(screen.getByLabelText("启用 OIDC"));
  await userEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() =>
    expect(api).toHaveBeenCalledWith(
      "/oidc",
      { ...config, enabled: true, client_secret: "test-only-secret" },
      "PUT",
    ),
  );
  await waitFor(() => expect((secret as HTMLInputElement).value).toBe(""));
  expect(await screen.findByText("OIDC 配置已保存。")).toBeTruthy();
  app.unmount();
  render(<OIDCSettings api={api as API} publicURL="https://room.example" />);
  expect(
    ((await screen.findByLabelText("Issuer")) as HTMLInputElement).value,
  ).toBe("https://auth.nodelane.net/oidc");
  expect((screen.getByLabelText("Client ID") as HTMLInputElement).value).toBe(
    "logto-app",
  );
  expect((screen.getByLabelText("启用 OIDC") as HTMLInputElement).checked).toBe(
    true,
  );
});
