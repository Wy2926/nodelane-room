// @vitest-environment jsdom
import {
  act,
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { UpdateSources } from "./update-sources";
import { UpdatePolicies } from "./update-policies";
import { Releases } from "./updates";
import { useUpdatesOverview } from "./update-data";
import type { API } from "./types";

afterEach(cleanup);
const empty = {
  sources: [],
  releases: [],
  policies: [],
  devices: [],
  versions: [],
  attempts: [],
  repository_revision: 0,
};

it("refresh preserves the source editor's original revision and never fills credentials", async () => {
  let revision = 1;
  const source = {
    id: "a".repeat(32),
    name: "R2 主源",
    kind: "r2",
    endpoint: "https://account.r2.cloudflarestorage.com",
    bucket: "releases",
    region: "auto",
    prefix: "",
    public_url: "",
    path_style: false,
    enabled: true,
    priority: 10,
    revision: 1,
    has_credentials: true,
  };
  const api = vi.fn(async (_path: string, _body?: unknown, method?: string) => {
    if (method === "PUT") throw new Error("状态已变化，请刷新后重试。");
    return {
      sources: [{ ...source, revision }],
      releases: [],
      policies: [],
      devices: [],
      versions: [],
      attempts: [],
      repository_revision: 0,
    };
  });
  render(<UpdateSources api={api as API} />);
  await userEvent.click(await screen.findByRole("button", { name: "配置" }));
  expect((screen.getByLabelText("Secret Key") as HTMLInputElement).value).toBe(
    "",
  );
  revision = 2;
  await userEvent.click(screen.getByRole("button", { name: "刷新" }));
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() =>
    expect(api).toHaveBeenCalledWith(
      "/updates/sources",
      expect.objectContaining({ revision: 1, secret_key: "", access_key: "" }),
      "PUT",
    ),
  );
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.getByLabelText("Secret Key")).toBeTruthy();
});

it("keeps a save conflict visible when an in-flight overview refresh completes", async () => {
  let finishRefresh!: (value: unknown) => void;
  let reads = 0;
  const source = {
    id: "primary",
    name: "主源",
    kind: "https",
    public_url: "https://cdn.example",
    revision: 1,
    priority: 10,
    enabled: true,
  };
  const api = vi.fn(async (_path: string, _body?: unknown, method?: string) => {
    if (method === "PUT") throw new Error("状态已变化，请刷新后重试。");
    if (++reads > 1)
      return new Promise((resolve) => {
        finishRefresh = resolve;
      });
    return { ...empty, sources: [source] };
  });
  render(<UpdateSources api={api as API} />);
  await userEvent.click(await screen.findByRole("button", { name: "配置" }));
  await userEvent.click(screen.getByRole("button", { name: "刷新" }));
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  await act(async () => {
    finishRefresh({ ...empty, sources: [{ ...source, revision: 2 }] });
  });
  expect(screen.getByRole("alert").textContent).toContain("状态已变化");
});

it("ignores a late overview response after its request was cancelled", async () => {
  let finishOld!: (value: unknown) => void;
  const oldAPI = vi.fn(
    () =>
      new Promise((resolve) => {
        finishOld = resolve;
      }),
  );
  const newAPI = vi.fn(async () => ({ ...empty, repository_revision: 2 }));
  const { result, rerender } = renderHook(
    ({ api }) => useUpdatesOverview(api),
    { initialProps: { api: oldAPI as API } },
  );
  rerender({ api: newAPI as API });
  await waitFor(() => expect(result.current.data?.repository_revision).toBe(2));
  await act(async () => {
    finishOld({ ...empty, repository_revision: 1 });
  });
  expect(result.current.data?.repository_revision).toBe(2);
});

it("keeps a policy edit tied to its original revision and offers only published matching releases", async () => {
  let revision = 3;
  const api = vi.fn(async (_path: string, _body?: unknown, method?: string) => {
    if (method === "PUT") throw new Error("状态已变化，请刷新后重试。");
    return {
      ...empty,
      policies: [
        {
          os: "windows",
          arch: "amd64",
          release_id: "valid",
          minimum_version: "",
          revision,
        },
      ],
      releases: [
        {
          id: "valid",
          version: "0.3.0",
          os: "windows",
          arch: "amd64",
          state: "published",
        },
        {
          id: "wrong-platform",
          version: "0.3.1",
          os: "linux",
          arch: "amd64",
          state: "published",
        },
        {
          id: "draft",
          version: "0.3.2",
          os: "windows",
          arch: "amd64",
          state: "draft",
        },
      ],
    };
  });
  render(<UpdatePolicies api={api as API} />);
  await userEvent.click(
    await screen.findByRole("button", { name: "配置 windows amd64" }),
  );
  expect(screen.getByRole("option", { name: "0.3.0" })).toBeTruthy();
  expect(screen.queryByRole("option", { name: "0.3.1" })).toBeNull();
  expect(screen.queryByRole("option", { name: "0.3.2" })).toBeNull();
  revision = 4;
  await userEvent.click(screen.getByRole("button", { name: "刷新" }));
  await userEvent.click(screen.getByRole("button", { name: "保存规则" }));
  await waitFor(() =>
    expect(api).toHaveBeenCalledWith(
      "/updates/policies",
      expect.objectContaining({ revision: 3, release_id: "valid" }),
      "PUT",
    ),
  );
  expect(screen.getByLabelText("推荐版本")).toBeTruthy();
});

it("validates the signed package size before an upload on the release page", async () => {
  const api = vi.fn(async () => ({
    ...empty,
    sources: [{ id: "primary", name: "主源" }],
    releases: [
      {
        id: "release",
        version: "0.3.0",
        os: "windows",
        arch: "amd64",
        state: "draft",
        size: 50,
        target: "nodelane-room.exe",
        notes: "",
        revision: 1,
        sha256: "test-digest",
        sources: [],
      },
    ],
  }));
  render(<Releases api={api as API} />);
  await userEvent.click(await screen.findByRole("button", { name: "管理" }));
  await userEvent.selectOptions(screen.getByLabelText("包存储源"), "primary");
  await userEvent.upload(
    screen.getByLabelText("完整安装包"),
    new File(["short"], "nodelane-room.exe", {
      type: "application/octet-stream",
    }),
  );
  await userEvent.click(screen.getByRole("button", { name: "上传并验证" }));
  expect(await screen.findByText("包大小与签名清单不符")).toBeTruthy();
  expect(api.mock.calls).toHaveLength(1);
});
