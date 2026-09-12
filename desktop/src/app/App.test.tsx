import { beforeEach, expect, test, vi } from "vitest";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App } from "./App";
import { rpc, exitApp, copyText, clientVersion } from "../native/api";
import { useService } from "../native/use-service";
import type { Game, Status } from "../shared/model";
import { languageStorageKey, setLanguage } from "../i18n";

vi.mock("../native/use-service", () => ({ useService: vi.fn() }));
vi.mock("../native/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../native/api")>()),
  rpc: vi.fn(),
  exitApp: vi.fn(),
  copyText: vi.fn(),
}));
vi.mock("@tauri-apps/api/core", () => ({
  isTauri: () => false,
  invoke: vi.fn(),
}));
const game: Game = {
  network: { version: 1, broadcast: true, multicast: true, ethernet_types: [] },
  id: "custom",
  name: "测试游戏",
  summary: "介绍",
  cover_url: "",
  background_url: "",
  source_url: "",
  enabled: true,
  revision: 1,
  ports: [{ protocol: "tcp", port: 25565 }],
};
let status: Status;
function defaultReply(request: { action: string }) {
  if (request.action === "games") return [game];
  if (request.action === "rooms") return { rooms: [], truncated: false };
  if (request.action === "capabilities")
    return { oidc_enabled: true, ready: true };
  if (request.action === "invite-info")
    return {
      active: true,
      revision: 1,
      expires_at: new Date(Date.now() + 600000).toISOString(),
    };
  if (request.action === "account-poll") return { state: "none" };
  return [];
}
beforeEach(() => {
  vi.clearAllMocks();
  setLanguage("zh-CN");
  status = {
    room_creation: { allowed: true },
    user: {
      id: "owner",
      name: "玩家",
      kind: "guest",
      state: "active",
      created_at: new Date().toISOString(),
    },
    version: "0.2.0",
    protocol_version: 3,
    service_instance_id: "test",
    status_seq: 1,
    service: "ready",
    identity: "active",
    operation: "idle",
    membership: { state: "none", revision: 0 },
    permissions: { manage: false, join: false, leave: false },
    network: { state: "stopped", generation: 0, applied_game_revision: 0 },
    freshness: {
      observed_at: new Date().toISOString(),
      snapshot_at: new Date().toISOString(),
    },
    issues: [],
    pending_operations: [],
    lan_version: 1,
    server: "https://example.test",
    name: "玩家",
    device_id: "owner",
    control: "online",
    engine: "stopped",
    selected_room: "",

    members: [],

    peers: [],
    snapshot_at: new Date().toISOString(),
  };
  vi.mocked(useService).mockImplementation(() => ({
    status,
    error: undefined,
    refresh: vi.fn(),
    updatedAt: Date.now(),
    refreshing: false,
    retryAt: 0,
    stale: false,
  }));
  vi.mocked(rpc).mockImplementation(
    async (request) => defaultReply(request) as never,
  );
});

test.each([null, "fr-FR", "invalid"])(
  "language must be chosen before the client starts: %s",
  async (saved) => {
    localStorage.removeItem(languageStorageKey);
    if (saved) localStorage.setItem(languageStorageKey, saved);
    const app = render(<App />);
    expect(
      screen.getByRole("heading", { name: /Choose your language/ }),
    ).toBeTruthy();
    expect(useService).not.toHaveBeenCalled();
    expect(rpc).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("radio", { name: "English" }));
    await userEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(screen.getByRole("button", { name: "Settings" })).toBeTruthy();
    expect(document.documentElement.lang).toBe("en-US");
    expect(localStorage.getItem(languageStorageKey)).toBe("en-US");
    app.unmount();
    render(<App />);
    expect(screen.queryByRole("radio")).toBeNull();
    expect(screen.getByRole("button", { name: "My rooms" })).toBeTruthy();
  },
);

test("Chinese choice enters initialization and settings can switch to English", async () => {
  localStorage.removeItem(languageStorageKey);
  status.device_id = "";
  render(<App />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "继续" }));
  expect(screen.getByRole("button", { name: "登录 / 注册" })).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "设置" }));
  await user.selectOptions(screen.getByRole("combobox"), "en-US");
  expect(
    screen.getByRole("button", { name: "Desktop preferences" }),
  ).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Device information" }));
  expect(screen.getByText("Device nickname")).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "My rooms" }));
  await user.click(screen.getByRole("button", { name: "Continue as a guest" }));
  await user.type(screen.getByLabelText("Nickname"), "Traveler");
  await user.click(screen.getByRole("button", { name: "Start playing" }));
  expect(rpc).toHaveBeenCalledWith({
    action: "init",
    server: "https://room.nodelane.net",
    name: "Traveler",
  });
});

test("an existing offline error follows the selected language", async () => {
  vi.mocked(useService).mockReturnValue({
    status: undefined,
    error: { code: "local_service_unavailable", error: "服务离线" },
    refresh: vi.fn(),
    updatedAt: 0,
    refreshing: false,
    retryAt: 0,
    stale: false,
  });
  render(<App />);
  await userEvent.click(screen.getByRole("button", { name: "设置" }));
  await userEvent.selectOptions(screen.getByRole("combobox"), "en-US");
  expect(screen.getByRole("alert").textContent).toContain(
    "The local service is unavailable",
  );
  expect(screen.getByRole("alert").textContent).not.toMatch(/\p{Script=Han}/u);
  await userEvent.click(screen.getByRole("button", { name: "Diagnostics" }));
  expect(
    screen.getAllByText("Cannot reach the local service").length,
  ).toBeGreaterThan(0);
  expect(rpc).not.toHaveBeenCalled();
});

test("catalog failure explains why creation is unavailable", async () => {
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "games") throw { code: "local_control_unreachable" };
    return defaultReply(request) as never;
  });
  render(<App />);
  await waitFor(() =>
    expect(screen.getByText(/无法连接联机服务/)).toBeTruthy(),
  );
  expect(
    (screen.getByRole("button", { name: "创建房间" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(
    (screen.getByRole("button", { name: "加入房间" }) as HTMLButtonElement)
      .disabled,
  ).toBe(false);
});

test("creation always uses the server general game revision without a selector", async () => {
  const games = [
    { ...game, id: "other", name: "Another game" },
    { ...game, revision: 7 },
  ];
  vi.mocked(rpc).mockImplementation(
    async (request) =>
      (request.action === "games" ? games : defaultReply(request)) as never,
  );
  render(<App />);
  const create = screen.getByRole("button", { name: "创建房间" });
  await waitFor(() =>
    expect((create as HTMLButtonElement).disabled).toBe(false),
  );
  await userEvent.click(create);
  expect(screen.queryByRole("combobox")).toBeNull();
  expect(document.activeElement).toBe(screen.getByLabelText("房间名称"));
  await userEvent.type(screen.getByLabelText("房间名称"), "周末一起玩");
  await userEvent.click(screen.getByRole("button", { name: "创建并连接" }));
  expect(rpc).toHaveBeenCalledWith(
    expect.objectContaining({
      action: "create",
      body: { game: "custom", name: "周末一起玩", expected_game_revision: 7 },
    }),
  );
});

test.each([false, true])(
  "general game unavailable does not fall back to another game: disabled=%s",
  async (disabled) => {
    const games = [
      { ...game, id: "other" },
      ...(disabled ? [{ ...game, enabled: false }] : []),
    ];
    vi.mocked(rpc).mockImplementation(
      async (request) =>
        (request.action === "games" ? games : defaultReply(request)) as never,
    );
    render(<App />);
    expect(
      await screen.findByText("通用房间暂不可用，请刷新后重试。"),
    ).toBeTruthy();
    expect(
      (screen.getByRole("button", { name: "创建房间" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(
      (screen.getByRole("button", { name: "加入房间" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);
  },
);

test("creation restriction preserves invitation joining and recovers without login", async () => {
  status.user!.state = "disabled";
  status.room_creation = { allowed: false, reason: "account_disabled" };
  const app = render(<App />);
  expect(
    (screen.getByRole("button", { name: "创建房间" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(screen.getAllByText(/当前账号暂不可创建房间/).length).toBeGreaterThan(
    0,
  );
  await userEvent.type(screen.getByLabelText("邀请码"), "test-code");
  await userEvent.click(screen.getByRole("button", { name: "加入房间" }));
  expect(rpc).toHaveBeenCalledWith(
    expect.objectContaining({ action: "join", body: { code: "test-code" } }),
  );
  await userEvent.click(screen.getByRole("button", { name: "返回我的房间" }));
  status = { ...status, room_creation: { allowed: true } };
  app.rerender(<App />);
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "创建房间" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
});

test("an already open creation form responds to a new restriction", async () => {
  const app = render(<App />);
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "创建房间" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  await userEvent.click(screen.getByRole("button", { name: "创建房间" }));
  await userEvent.type(screen.getByLabelText("房间名称"), "未提交房间");
  status = {
    ...status,
    room_creation: { allowed: false, reason: "account_disabled" },
  };
  app.rerender(<App />);
  const submit = screen.getByRole("button", {
    name: "创建并连接",
  }) as HTMLButtonElement;
  expect(submit.disabled).toBe(true);
  fireEvent.submit(submit.closest("form")!);
  expect(vi.mocked(rpc).mock.calls.some(([r]) => r.action === "create")).toBe(
    false,
  );
});

test("first use uses the online control endpoint without a server setting", async () => {
  status.device_id = "";
  status.server = "";
  const { container } = render(<App />);
  expect(
    container.querySelector('input[type="url"], input[name="server"]'),
  ).toBeNull();
  const user = userEvent.setup();
  await user.click(
    screen.getByRole("button", { name: "暂不登录，以访客开始" }),
  );
  await user.type(screen.getByLabelText("昵称"), "旅人");
  await user.click(screen.getByRole("button", { name: "开始联机" }));
  expect(rpc).toHaveBeenCalledWith({
    action: "init",
    server: "https://room.nodelane.net",
    name: "旅人",
  });
});

test("welcome signs in directly without first creating a guest", async () => {
  status.device_id = "";
  status.user = undefined;
  render(<App />);
  expect(screen.queryByRole("textbox")).toBeNull();
  const login = screen.getByRole("button", { name: "登录 / 注册" });
  await waitFor(() =>
    expect((login as HTMLButtonElement).disabled).toBe(false),
  );
  await userEvent.click(login);
  expect(rpc).toHaveBeenCalledWith({
    action: "account-login",
    server: "https://example.test",
    name: "玩家",
  });
  expect(
    vi.mocked(rpc).mock.calls.some(([request]) => request.action === "init"),
  ).toBe(false);
});

test("welcome waiting state offers resume and cancel without guest initialization", async () => {
  status.device_id = "";
  status.user = undefined;
  vi.mocked(rpc).mockImplementation(
    async (request) =>
      (request.action === "account-poll"
        ? { state: "waiting" }
        : defaultReply(request)) as never,
  );
  render(<App />);
  await userEvent.click(
    await screen.findByRole("button", { name: "打开登录页面" }),
  );
  expect(rpc).toHaveBeenCalledWith(
    expect.objectContaining({ action: "account-login" }),
  );
  expect(
    screen.queryByRole("button", { name: "暂不登录，以访客开始" }),
  ).toBeNull();
  expect(screen.queryByRole("textbox")).toBeNull();
  await userEvent.click(screen.getByRole("button", { name: "取消等待" }));
  expect(rpc).toHaveBeenCalledWith({ action: "account-cancel" });
});

test("registered account separates device access and confirms revocation", async () => {
  status.user!.kind = "registered";
  vi.mocked(rpc).mockImplementation(
    async (request) =>
      (request.action === "account-devices"
        ? [
            { device_id: "owner", name: "本机", revoked: false },
            { device_id: "laptop", name: "笔记本", revoked: false },
            { device_id: "old", name: "旧电脑", revoked: true },
          ]
        : defaultReply(request)) as never,
  );
  render(<App />);
  await userEvent.click(screen.getByRole("button", { name: "设置" }));
  await userEvent.click(screen.getByRole("button", { name: "账号" }));
  const panel = screen.getByRole("region", { name: "账号" });
  expect(within(panel).getByText("账号信息").closest("details")!.open).toBe(
    false,
  );
  const devices = await within(panel).findByText("已授权设备");
  expect(devices.closest("details")!.open).toBe(false);
  await userEvent.click(devices);
  expect(within(panel).getByText("当前设备")).toBeTruthy();
  expect(within(panel).getByText("已撤销授权")).toBeTruthy();
  const revoke = within(panel).getAllByRole("button", { name: "撤销设备授权" });
  expect(revoke).toHaveLength(1);
  await userEvent.click(revoke[0]);
  expect(screen.getByRole("dialog").textContent).toContain("笔记本");
  expect(
    vi
      .mocked(rpc)
      .mock.calls.some(([request]) => request.action === "revoke-device"),
  ).toBe(false);
  await userEvent.click(
    screen.getByRole("button", { name: "确认撤销设备授权" }),
  );
  expect(rpc).toHaveBeenCalledWith(
    expect.objectContaining({ action: "revoke-device", target: "laptop" }),
  );
});

test("account login has no server setting and uses the existing identity server", async () => {
  const { container } = render(<App />);
  await userEvent.click(screen.getByRole("button", { name: "设置" }));
  await userEvent.click(screen.getByRole("button", { name: "账号" }));
  expect(
    container.querySelector('input[type="url"], input[name="server"]'),
  ).toBeNull();
  await waitFor(() =>
    expect(rpc).toHaveBeenCalledWith({
      action: "capabilities",
      server: "https://example.test",
    }),
  );
  await userEvent.click(screen.getByRole("button", { name: "登录已有账号" }));
  await userEvent.click(
    screen.getByRole("button", { name: "确认登录已有账号" }),
  );
  expect(rpc).toHaveBeenCalledWith({
    action: "account-login",
    server: "https://example.test",
    name: "玩家",
  });
});

test("settings and diagnostics work before identity initialization", async () => {
  status.device_id = "";
  render(<App />);
  await userEvent.click(screen.getByRole("button", { name: "设置" }));
  await userEvent.click(screen.getByRole("button", { name: "版本与更新" }));
  expect(screen.getByText(/当前版本/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "开始联机" })).toBeNull();
  await userEvent.click(screen.getByRole("button", { name: "网络诊断" }));
  expect(screen.queryByRole("button", { name: "开始联机" })).toBeNull();
  await userEvent.click(screen.getByRole("button", { name: "我的房间" }));
  expect(screen.getByRole("button", { name: "登录 / 注册" })).toBeTruthy();
});

test("renewed invitation uses the actual standalone invitation response", async () => {
  status.selected_room = "room";
  status.room = {
    id: "room",
    name: "联机房间",
    game: game.id,
    game_name: game.name,
    owner_user_id: "owner",
    revision: 1,
    capacity: 4,
    closed: false,
    expires_at: new Date(Date.now() + 3600000).toISOString(),
  };
  status.game = game;
  status.permissions = { manage: true, join: false, leave: true };
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "invite")
      return {
        code: "test-invitation",
        revision: 1,
        expires_at: new Date(Date.now() + 600000).toISOString(),
      } as never;
    return defaultReply(request) as never;
  });
  render(<App />);
  await userEvent.click(screen.getByRole("button", { name: "邀请朋友" }));
  expect(await screen.findByRole("dialog")).toBeTruthy();
  expect(await screen.findByText("test-invitation")).toBeTruthy();
  expect(screen.getByRole("button", { name: "复制邀请码" })).toBeTruthy();
  expect(rpc).toHaveBeenCalledWith({
    action: "invite",
    room: "room",
    body: { expected_revision: 1 },
    command_id: expect.any(String),
  });
});

test("an open join form stops accepting operations when the service disappears", async () => {
  const app = render(<App />);
  await userEvent.type(screen.getByLabelText("邀请码"), "test-code");
  vi.mocked(useService).mockReturnValue({
    status,
    error: { code: "local_service_unavailable", error: "服务离线" },
    refresh: vi.fn(),
    updatedAt: Date.now(),
    refreshing: false,
    retryAt: 0,
    stale: false,
  });
  app.rerender(<App />);
  expect(
    (screen.getByRole("button", { name: "加入房间" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(
    vi.mocked(rpc).mock.calls.some(([request]) => request.action === "join"),
  ).toBe(false);
});

test("configured game ports are read-only and stop state is explicit", async () => {
  status.selected_room = "room";
  status.room = {
    id: "room",
    name: "房间",
    owner_user_id: "owner",
    game: game.id,
    game_name: game.name,
    revision: 1,
    capacity: 4,
    closed: false,
    expires_at: new Date(Date.now() + 3600000).toISOString(),
  };
  status.game = { ...game, enabled: false };
  status.control = "online";
  status.engine = "running";
  render(<App />);
  expect(screen.getByText(/此游戏已被管理员停用/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "添加" })).toBeNull();
  expect(screen.queryByRole("button", { name: "删除" })).toBeNull();
  expect(screen.getByText(/配置由管理员维护/)).toBeTruthy();
});

test("leave failure keeps the app running", async () => {
  status.selected_room = "room";
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "leave")
      throw { code: "local_rpc_timeout", error: "timeout" };
    return defaultReply(request) as never;
  });
  render(<App />);
  const user = userEvent.setup();
  await user.click(screen.getByLabelText("个人资料"));
  await user.click(screen.getByRole("button", { name: /^离房并退出/ }));
  await user.click(screen.getByRole("button", { name: "确认离房并退出" }));
  expect(await screen.findByText("正在确认操作结果")).toBeTruthy();
  expect(exitApp).not.toHaveBeenCalled();
  expect(screen.getByRole("dialog")).toBeTruthy();
});

test("creating a room submits the general server game once and retains invitation only in memory", async () => {
  let complete!: (value: unknown) => void;
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "create")
      return (await new Promise<unknown>((resolve) => {
        complete = resolve;
      })) as never;
    return defaultReply(request) as never;
  });
  render(<App />);
  const user = userEvent.setup();
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "创建房间" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  await user.click(await screen.findByRole("button", { name: "创建房间" }));
  await user.type(screen.getByLabelText("房间名称"), "周末世界");
  const submit = screen.getByRole("button", { name: "创建并连接" });
  fireEvent.submit(submit.closest("form")!);
  fireEvent.submit(submit.closest("form")!);
  const creates = vi
    .mocked(rpc)
    .mock.calls.filter(([r]) => r.action === "create");
  expect(creates).toHaveLength(1);
  expect(creates[0][0].body).toEqual({
    name: "周末世界",
    game: game.id,
    expected_game_revision: 1,
  });
  complete({
    room: { name: "周末世界", game_name: game.name },
    invitation: {
      code: "fixture-invitation",
      revision: 1,
      expires_at: new Date(Date.now() + 100000).toISOString(),
    },
  });
  await waitFor(() =>
    expect(screen.getByText("fixture-invitation")).toBeTruthy(),
  );
  expect(Object.keys(localStorage)).toEqual([languageStorageKey]);
  expect(localStorage.getItem(languageStorageKey)).toBe("zh-CN");
  await user.click(screen.getByRole("button", { name: "关闭对话框" }));
  expect(screen.queryByText("fixture-invitation")).toBeNull();
});

function joinedParty() {
  status.selected_room = "room";
  status.room = {
    id: "room",
    name: "周末世界",
    owner_user_id: "owner",
    game: game.id,
    game_name: game.name,
    revision: 1,
    capacity: 4,
    closed: false,
    expires_at: new Date(Date.now() + 3600000).toISOString(),
  };
  status.game = game;
  status.permissions = { manage: true, join: false, leave: true };
  status.control = "online";
  status.engine = "running";
  status.ip = "10.203.0.2";
  status.lease_expires_at = new Date(Date.now() + 60000).toISOString();
  status.membership = {
    state: "active",
    revision: 1,
    valid_until: new Date(Date.now() + 60000).toISOString(),
  };
  status.members = [
    {
      device_id: "owner",
      user_id: "owner",
      name: "玩家",
      ip: "10.203.0.2",
      last_seen: new Date().toISOString(),
    },
    {
      device_id: "guest",
      user_id: "guest",
      name: "远山",
      ip: "10.203.0.3",
      last_seen: new Date().toISOString(),
    },
  ];
}

test("English room actions and diagnostics use complete translated messages", async () => {
  setLanguage("en-US");
  joinedParty();
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "doctor")
      return {
        control: "online",
        engine: "running",
        platform: {
          os: "windows",
          tap_interface_present: true,
          interfaces: [],
        },
      } as never;
    return defaultReply(request) as never;
  });
  render(<App />);
  const user = userEvent.setup();
  expect(screen.getByText(/2 \/ 4/)).toBeTruthy();
  expect(screen.getByText(/^Expires /)).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Leave room" }));
  expect(
    screen.getByRole("button", { name: "Confirm: Leave room" }),
  ).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Cancel" }));
  await user.click(screen.getByRole("button", { name: "Diagnostics" }));
  expect(screen.queryByRole("button", { name: "Run diagnostics" })).toBeNull();
  expect(await screen.findByText("Windows")).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: "Copy redacted diagnostics" }),
  ).toBeNull();
  expect(document.querySelector("main")!.textContent).not.toMatch(
    /\p{Script=Han}|\{\w+\}/u,
  );
});

test("party management disclosure supports Escape and retains confirmation before transfer", async () => {
  joinedParty();
  render(<App />);
  const user = userEvent.setup();
  expect(screen.queryByLabelText("管理 玩家")).toBeNull();
  const manage = screen.getByLabelText("管理 远山");
  expect(manage.closest("details")!.open).toBe(false);
  await user.click(manage);
  expect(screen.getByRole("button", { name: "转让房主" })).toBeTruthy();
  await user.keyboard("{Escape}");
  expect(manage.closest("details")!.open).toBe(false);
  expect(document.activeElement).toBe(manage);
  await user.click(manage);
  await user.click(screen.getByRole("button", { name: "转让房主" }));
  expect(screen.getByRole("dialog").textContent).toContain("远山");
  expect(vi.mocked(rpc).mock.calls.some(([r]) => r.action === "transfer")).toBe(
    false,
  );
  await user.click(screen.getByRole("button", { name: "关闭对话框" }));
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: "转让房主" }),
  );
  await user.keyboard("{Escape}");
  expect(manage.closest("details")!.open).toBe(false);
  expect(document.activeElement).toBe(manage);
  await user.click(manage);
  await user.click(screen.getByRole("button", { name: "转让房主" }));
  await user.click(screen.getByRole("button", { name: "确认转让房主" }));
  expect(rpc).toHaveBeenCalledWith({
    action: "transfer",
    room: "room",
    body: { device_id: "guest", expected_revision: 1 },
    command_id: expect.any(String),
  });
});

test.each([false, true])(
  "member cards keep actual zero measurements and reject stale actions: stale=%s",
  async (stale) => {
    joinedParty();
    status.peers = [
      {
        device_id: "guest",
        name: "远山",
        ip: "10.203.0.3",
        mode: "direct",
        measured_at: new Date().toISOString(),
        rtt_ms: 0,
        loss_percent: 0,
      },
    ];
    if (stale) status.snapshot_at = new Date(Date.now() - 60000).toISOString();
    render(<App />);
    expect(screen.getByText(stale ? "链路未知" : "直连")).toBeTruthy();
    expect(screen.queryByText("0.0 ms") !== null).toBe(!stale);
    expect(screen.queryByText("0.0%") !== null).toBe(!stale);
    await userEvent.click(screen.getByLabelText("管理 远山"));
    expect(
      (screen.getByRole("button", { name: "转让房主" }) as HTMLButtonElement)
        .disabled,
    ).toBe(stale);
    expect(
      (screen.getByRole("button", { name: "踢出成员" }) as HTMLButtonElement)
        .disabled,
    ).toBe(stale);
    expect(screen.queryByRole("button", { name: "测延迟" })).toBeNull();
  },
);

test("offline startup keeps updates reachable and reports the local service failure", async () => {
  vi.mocked(rpc).mockRejectedValue({
    code: "local_service_unavailable",
    error: "服务离线",
  });
  vi.mocked(useService).mockReturnValue({
    status: undefined,
    error: { code: "local_service_unavailable", error: "服务离线" },
    refresh: vi.fn(),
    updatedAt: 0,
    refreshing: false,
    retryAt: 0,
    stale: false,
  });
  render(<App />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "设置" }));
  await user.click(screen.getByRole("button", { name: "版本与更新" }));
  await user.click(screen.getByRole("button", { name: "检查更新" }));
  expect(
    screen
      .getAllByRole("alert")
      .some((v) => v.textContent?.includes("无法连接本机服务")),
  ).toBe(true);
  expect(screen.queryByText("已是最新版")).toBeNull();
  expect(rpc).toHaveBeenCalledWith({ action: "update-check" });
  await user.click(screen.getByRole("button", { name: "网络诊断" }));
  expect(screen.getAllByText("无法连接本机服务").length).toBeGreaterThan(0);
  expect(screen.queryByRole("button", { name: "运行诊断" })).toBeNull();
  expect(rpc).not.toHaveBeenCalledWith({ action: "doctor" });
});

test("diagnostics shows system checks without interfaces, peer data, or raw reports", async () => {
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "doctor")
      return {
        control: "online",
        engine: "stopped",
        nebula_version: "1.11.1",
        device_id: "private-id",
        error: "private-error",
        virtual_ip: "10.203.0.7",
        unexpected_secret: "private-value",
        peers: [{ name: "private-peer" }],
        platform: {
          os: "windows",
          arch: "amd64",
          tap_interface_present: false,
          private_path: "private-path",
          interfaces: [
            {
              name: "private-interface",
              up: false,
              mtu: 1500,
              addresses: ["192.0.2.9/24"],
            },
          ],
        },
      } as never;
    return defaultReply(request) as never;
  });
  render(<App />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "网络诊断" }));
  expect(screen.queryByRole("button", { name: "运行诊断" })).toBeNull();
  expect(await screen.findByText("Windows")).toBeTruthy();
  expect(screen.getByText("未找到")).toBeTruthy();
  expect(screen.getByText("1.11.1")).toBeTruthy();
  expect(document.querySelector("pre")).toBeNull();
  expect(screen.queryByText("查看网络接口")).toBeNull();
  expect(screen.queryByText("成员链路")).toBeNull();
  expect(screen.queryByRole("button", { name: "复制脱敏诊断" })).toBeNull();
  expect(document.querySelector("main")!.textContent).not.toMatch(
    /private-|192\.0\.2|unexpected_secret/,
  );
  expect(copyText).not.toHaveBeenCalled();
});

test.each([
  "fresh",
  "stale",
  "expired",
  "offline",
  "unlinked",
  "unmeasured",
  "old",
  "window",
  "lost",
])("member metrics respect actual rolling measurements: %s", async (state) => {
  joinedParty();
  status.lease_expires_at = new Date(
    Date.now() + (state === "expired" ? -1000 : 60000),
  ).toISOString();
  status.ip = "10.203.0.2";
  status.peers = [
    {
      device_id: "guest",
      name: "远山",
      ip: "10.203.0.3",
      measured_at: new Date().toISOString(),
      mode: ["unlinked", "lost"].includes(state) ? "unknown" : "direct",
      rtt_ms: ["unmeasured", "lost"].includes(state) ? undefined : 0,
      loss_percent:
        state === "unmeasured" ? undefined : state === "lost" ? 100 : 0,
    },
  ];
  if (state === "stale")
    status.snapshot_at = new Date(Date.now() - 60000).toISOString();
  if (state === "offline")
    vi.mocked(useService).mockReturnValue({
      status,
      error: { code: "local_service_unavailable", error: "服务离线" },
      refresh: vi.fn(),
      updatedAt: Date.now(),
      refreshing: false,
      retryAt: 0,
      stale: false,
    });
  if (["old", "window"].includes(state))
    status.peers[0].measured_at = new Date(
      Date.now() - (state === "old" ? 31000 : 20000),
    ).toISOString();
  render(<App />);
  if (["fresh", "unlinked", "window"].includes(state)) {
    expect(screen.getByText("0.0 ms")).toBeTruthy();
    expect(screen.getByText("0.0%")).toBeTruthy();
  } else {
    expect(screen.queryByText("0.0 ms")).toBeNull();
  }
  if (state === "lost") expect(screen.getByText("100.0%")).toBeTruthy();
  expect(vi.mocked(rpc).mock.calls.some(([r]) => r.action === "ping")).toBe(
    false,
  );
  await userEvent.click(screen.getByRole("button", { name: "网络诊断" }));
  expect(screen.queryByText("0.0 ms")).toBeNull();
  expect(screen.queryByText("成员链路")).toBeNull();
});

test("an idle device with a zero lease does not display an expired authorization", async () => {
  status.lease_expires_at = "0001-01-01T00:00:00Z";
  render(<App />);
  await userEvent.click(screen.getByRole("button", { name: "网络诊断" }));
  expect(screen.getByText("暂无授权")).toBeTruthy();
  expect(screen.queryByText("授权已到期")).toBeNull();
});

test("guest binding preserves the current account and does not offer logout", async () => {
  render(<App />);
  await userEvent.click(screen.getByRole("button", { name: "设置" }));
  await userEvent.click(screen.getByRole("button", { name: "账号" }));
  expect(screen.queryByRole("button", { name: "退出账号" })).toBeNull();
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "绑定账号" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false),
  );
  await userEvent.click(screen.getByRole("button", { name: "绑定账号" }));
  expect(rpc).toHaveBeenCalledWith({ action: "account-link" });
  expect(rpc).not.toHaveBeenCalledWith(
    expect.objectContaining({ action: "init" }),
  );
});

test("a restricted owner can still invite and leave an existing room", async () => {
  joinedParty();
  status.user!.state = "disabled";
  status.room_creation = { allowed: false, reason: "account_disabled" };
  render(<App />);
  expect(
    (screen.getByRole("button", { name: "邀请朋友" }) as HTMLButtonElement)
      .disabled,
  ).toBe(false);
  await userEvent.click(screen.getByRole("button", { name: "离开房间" }));
  await userEvent.click(screen.getByRole("button", { name: "确认离开房间" }));
  expect(rpc).toHaveBeenCalledWith(
    expect.objectContaining({ action: "leave", room: "room" }),
  );
});

test("profile menu opens account settings and closes with Escape", async () => {
  render(<App />);
  const profile = screen.getByLabelText("个人资料");
  await userEvent.click(profile);
  expect(screen.getByRole("button", { name: /^离房并退出/ })).toBeTruthy();
  await userEvent.keyboard("{Escape}");
  expect(profile.closest("details")!.open).toBe(false);
  expect(document.activeElement).toBe(profile);
  await userEvent.click(profile);
  await userEvent.click(screen.getByRole("button", { name: "账号" }));
  expect(screen.getByRole("region", { name: "账号" })).toBeTruthy();
});

test("a signed out account exposes login without exposing room creation", async () => {
  status.control = "signed_out";
  status.identity = "signed_out";
  status.user = undefined;
  render(<App />);
  await userEvent.click(screen.getByRole("button", { name: "我的房间" }));
  expect(screen.getByRole("button", { name: "登录 / 注册" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "创建房间" })).toBeNull();
  await waitFor(() =>
    expect(
      (
        screen.getByRole("button", {
          name: "登录 / 注册",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  await userEvent.click(screen.getByRole("button", { name: "登录 / 注册" }));
  expect(rpc).toHaveBeenCalledWith(
    expect.objectContaining({ action: "account-login" }),
  );
});

test("room detail reserves the sidebar for game information and actions", () => {
  joinedParty();
  render(<App />);
  expect(screen.queryByPlaceholderText("粘贴邀请码")).toBeNull();
  expect(screen.queryByRole("button", { name: "创建房间" })).toBeNull();
  expect(screen.queryByText("暂停本机网络")).toBeNull();
  const table = screen.getByRole("table");
  expect(table.querySelectorAll("th")).toHaveLength(6);
  for (const row of table.querySelectorAll("tbody tr"))
    expect(row.querySelectorAll("td")).toHaveLength(6);
  expect(table.closest(".room-main")).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "邀请朋友" }).closest("aside"),
  ).toBeTruthy();
});

test("client version is shown in the header and service version is absent from settings", async () => {
  status.version = "99.88.77";
  render(<App />);
  expect(
    within(screen.getByRole("banner")).getByText(`v${clientVersion}`),
  ).toBeTruthy();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "设置" }));
  for (const category of ["设备信息", "版本与更新"]) {
    await user.click(screen.getByRole("button", { name: category }));
    expect(screen.queryByText("后台版本")).toBeNull();
    expect(screen.queryByText("99.88.77")).toBeNull();
  }
});

test("current room is first even when owned catalog order differs, with no duplicate", async () => {
  joinedParty();
  const current = status.room!;
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "rooms")
      return {
        rooms: [
          { ...current, id: "other", name: "另一间房间" },
          { ...current, name: "旧快照房名" },
        ],
        truncated: false,
      } as never;
    return defaultReply(request) as never;
  });
  render(<App />);
  await userEvent.click(screen.getByRole("button", { name: "返回我的房间" }));
  const rooms = await screen.findAllByRole("article");
  expect(rooms).toHaveLength(2);
  expect(within(rooms[0]).getByRole("heading").textContent).toBe(current.name);
  expect(rooms[0].getAttribute("data-current")).toBe("true");
  expect(rooms[1].getAttribute("data-current")).toBe("false");
  expect(screen.queryByText("旧快照房名")).toBeNull();
});

test("diagnostics refreshes on entry and ignores a previous service response", async () => {
  let finishOld: (value: unknown) => void = () => {};
  let count = 0;
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "doctor") {
      count++;
      if (count === 1)
        return (await new Promise<unknown>((resolve) => {
          finishOld = resolve;
        })) as never;
      return {
        platform: { os: "linux", arch: "amd64", tun_device_present: true },
      } as never;
    }
    return defaultReply(request) as never;
  });
  const app = render(<App />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "网络诊断" }));
  expect(count).toBe(1);
  expect(screen.getByText("正在检查…")).toBeTruthy();
  status = { ...status, service_instance_id: "replacement" };
  app.rerender(<App />);
  expect(await screen.findByText("Linux")).toBeTruthy();
  finishOld({ platform: { os: "windows" } });
  await user.click(screen.getByRole("button", { name: "我的房间" }));
  await user.click(screen.getByRole("button", { name: "网络诊断" }));
  expect(await screen.findByText("Linux")).toBeTruthy();
  expect(screen.queryByText("Windows")).toBeNull();
  expect(count).toBe(3);
});
