// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Updates } from "./updates";
import type { API } from "./types";

afterEach(cleanup);
it("refresh preserves the source editor's original revision and never fills credentials", async () => {
  let revision = 1;
  const source = {id: "a".repeat(32), name: "R2 主源", kind: "r2", endpoint: "https://account.r2.cloudflarestorage.com", bucket: "releases", region: "auto", prefix: "", public_url: "", path_style: false, enabled: true, priority: 10, revision: 1, has_credentials: true};
  const api = vi.fn(async (_path: string, _body?: unknown, method?: string) => {
    if (method === "PUT") throw new Error("状态已变化，请刷新后重试。");
    return {sources: [{...source, revision}], releases: [], policies: [], devices: [], versions: [], attempts: [], repository_revision: 0};
  });
  render(<Updates api={api as API} />);
  await userEvent.click(await screen.findByRole("button", {name: "存储源"}));
  await userEvent.click(screen.getByRole("button", {name: "配置"}));
  expect((screen.getByLabelText("Secret Key") as HTMLInputElement).value).toBe("");
  revision = 2;
  await userEvent.click(screen.getByRole("button", {name: "刷新"}));
  await userEvent.click(screen.getByRole("button", {name: "保存"}));
  await waitFor(() => expect(api).toHaveBeenCalledWith("/updates/sources", expect.objectContaining({revision: 1, secret_key: "", access_key: ""}), "PUT"));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.getByLabelText("Secret Key")).toBeTruthy();
});
