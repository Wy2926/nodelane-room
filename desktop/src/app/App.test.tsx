import { beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App } from "./App";
import { rpc, exitApp, copyText } from "../native/api";
import { useService } from "../native/use-service";
import type { Game, Status } from "../shared/model";

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
  id: "configured",
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
beforeEach(() => {
  vi.clearAllMocks();
  status = {
    version: "0.2.0",
    protocol_version: 2,
    lan_version: 1,
    server: "https://example.test",
    name: "玩家",
    device_id: "owner",
    control: "idle",
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
  }));
  vi.mocked(rpc).mockImplementation(
    async (request) => (request.action === "games" ? [game] : []) as never,
  );
});

test("catalog failure remains a failure and cannot create from stale data", async () => {
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "games")
      throw { code: "control_unavailable", error: "连接失败" };
    return [] as never;
  });
  render(<App />);
  await userEvent.click(screen.getByRole("button", { name: /游戏库/ }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.getByText("游戏库暂不可用")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "创建房间" })).toBeNull();
});

test("console shelf reaches games beyond eight by keyboard and search resets to a valid selection", async () => {
  const games = Array.from({ length: 10 }, (_, index) => ({
    ...game,
    id: `game-${index}`,
    name: `冒险 ${index}`,
  }));
  vi.mocked(rpc).mockImplementation(
    async (request) => (request.action === "games" ? games : []) as never,
  );
  render(<App />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "游戏库" }));
  const first = await screen.findByRole("button", { name: "冒险 0" });
  first.focus();
  await user.keyboard("{End}");
  const last = screen.getByRole("button", { name: "冒险 9" });
  expect(document.activeElement).toBe(last);
  expect(last.getAttribute("aria-pressed")).toBe("true");
  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(first);
  await user.type(
    screen.getByRole("searchbox", { name: "搜索游戏" }),
    "冒险 4",
  );
  expect(
    screen.getByRole("button", { name: "冒险 4" }).getAttribute("aria-pressed"),
  ).toBe("true");
  await user.click(screen.getByRole("button", { name: "创建房间" }));
  expect(screen.getByRole("dialog").textContent).toContain("冒险 4");
});

test("first use submits the online control endpoint by default", async () => {
  status.device_id = "";
  render(<App />);
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("设备昵称"), "旅人");
  await user.click(screen.getByRole("button", { name: "开始旅程" }));
  expect(rpc).toHaveBeenCalledWith({
    action: "init",
    server: "https://room.nodelane.net",
    name: "旅人",
  });
});

test("settings and diagnostics work before identity initialization", async () => {
  status.device_id = "";
  render(<App />);
  await userEvent.click(screen.getByRole("button", { name: "设置" }));
  await userEvent.click(screen.getByRole("button", { name: "版本与更新" }));
  expect(screen.getByText(/当前版本/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "开始旅程" })).toBeNull();
  await userEvent.click(screen.getByRole("button", { name: "网络诊断" }));
  expect(screen.queryByRole("button", { name: "开始旅程" })).toBeNull();
  await userEvent.click(screen.getByRole("button", { name: "我的房间" }));
  expect(screen.getByRole("button", { name: "开始旅程" })).toBeTruthy();
});

test("renewed invitation uses the actual standalone invitation response", async () => {
  status.selected_room = "room";
  status.room = {
    id: "room",
    name: "联机房间",
    game: game.id,
    game_name: game.name,
    owner_id: "owner",
    revision: 1,
    capacity: 32,
    closed: false,
    expires_at: new Date(Date.now() + 3600000).toISOString(),
  };
  status.game = game;
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "invite")
      return {
        code: "test-invitation",
        expires_at: new Date(Date.now() + 600000).toISOString(),
      } as never;
    return (request.action === "games" ? [game] : []) as never;
  });
  render(<App />);
  await userEvent.click(screen.getByRole("button", { name: "生成新邀请码" }));
  expect(await screen.findByRole("dialog")).toBeTruthy();
  expect(screen.getByText("test-invitation")).toBeTruthy();
  expect(screen.getByRole("button", { name: "复制邀请码" })).toBeTruthy();
  expect(rpc).toHaveBeenCalledWith({ action: "invite", room: "room" });
});

test("an open join form stops accepting operations when the service disappears", async () => {
  const app = render(<App />);
  await userEvent.click(screen.getByRole("button", { name: /邀请码入房/ }));
  await userEvent.type(screen.getByLabelText("邀请码"), "test-code");
  vi.mocked(useService).mockReturnValue({
    status,
    error: { code: "service_unavailable", error: "服务离线" },
    refresh: vi.fn(),
    updatedAt: Date.now(),
  });
  app.rerender(<App />);
  expect(
    (screen.getByRole("button", { name: "加入并连接" }) as HTMLButtonElement)
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
    owner_id: "owner",
    game: game.id,
    game_name: game.name,
    revision: 1,
    capacity: 32,
    closed: false,
    expires_at: new Date(Date.now() + 3600000).toISOString(),
  };
  status.game = { ...game, enabled: false };
  status.control = "connected";
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
    if (request.action === "leave") throw { code: "timeout", error: "timeout" };
    return [] as never;
  });
  render(<App />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: /设置/ }));
  await user.click(screen.getByRole("button", { name: "退出与联机" }));
  await user.click(screen.getByRole("button", { name: "离房并退出" }));
  await user.click(screen.getByRole("button", { name: "确认离房并退出" }));
  expect(await screen.findByText(/请求超时，操作结果尚未确认/)).toBeTruthy();
  expect(exitApp).not.toHaveBeenCalled();
  expect(screen.getByRole("dialog")).toBeTruthy();
});

test("creating a room submits the selected server game once and retains invitation only in memory", async () => {
  let complete!: (value: unknown) => void;
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "create")
      return (await new Promise<unknown>((resolve) => {
        complete = resolve;
      })) as never;
    return (request.action === "games" ? [game] : []) as never;
  });
  render(<App />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: /游戏库/ }));
  await user.click(await screen.findByRole("button", { name: "创建房间" }));
  await user.type(screen.getByLabelText("房间名称"), "周末世界");
  const submit = screen.getByRole("button", { name: "创建并连接" });
  fireEvent.submit(submit.closest("form")!);
  fireEvent.submit(submit.closest("form")!);
  const creates = vi
    .mocked(rpc)
    .mock.calls.filter(([r]) => r.action === "create");
  expect(creates).toHaveLength(1);
  expect(creates[0][0].body).toEqual({ name: "周末世界", game: game.id });
  complete({
    room: { name: "周末世界", game_name: game.name },
    invitation: {
      code: "fixture-invitation",
      expires_at: new Date(Date.now() + 100000).toISOString(),
    },
  });
  await waitFor(() =>
    expect(screen.getByText("fixture-invitation")).toBeTruthy(),
  );
  expect(localStorage.length).toBe(0);
  await user.click(screen.getByRole("button", { name: "关闭对话框" }));
  expect(screen.queryByText("fixture-invitation")).toBeNull();
});

function joinedParty() {
  status.selected_room = "room";
  status.room = {
    id: "room",
    name: "周末世界",
    owner_id: "owner",
    game: game.id,
    game_name: game.name,
    revision: 1,
    capacity: 8,
    closed: false,
    expires_at: new Date(Date.now() + 3600000).toISOString(),
  };
  status.game = game;
  status.control = "connected";
  status.engine = "running";
  status.members = [
    {
      device_id: "owner",
      name: "玩家",
      ip: "10.203.0.2",
      last_seen: new Date().toISOString(),
    },
    {
      device_id: "guest",
      name: "远山",
      ip: "10.203.0.3",
      last_seen: new Date().toISOString(),
    },
  ];
}

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
    body: { device_id: "guest" },
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
        rtt_ms: 0,
        loss_percent: 0,
      },
    ];
    if (stale) status.snapshot_at = new Date(Date.now() - 60000).toISOString();
    render(<App />);
    expect(screen.getByText(stale ? "链路未知" : "直连")).toBeTruthy();
    expect(
      screen.getByText(stale ? "暂无实测数据" : "0.0 ms · 0% 丢包"),
    ).toBeTruthy();
    await userEvent.click(screen.getByLabelText("管理 远山"));
    expect(
      (screen.getByRole("button", { name: "转让房主" }) as HTMLButtonElement)
        .disabled,
    ).toBe(stale);
    expect(
      (screen.getByRole("button", { name: "踢出成员" }) as HTMLButtonElement)
        .disabled,
    ).toBe(stale);
    expect(
      (screen.getByRole("button", { name: "测延迟" }) as HTMLButtonElement)
        .disabled,
    ).toBe(stale);
  },
);

test("offline startup keeps settings and the update placeholder reachable without network requests", async () => {
  vi.mocked(useService).mockReturnValue({
    status: undefined,
    error: { code: "service_unavailable", error: "服务离线" },
    refresh: vi.fn(),
    updatedAt: 0,
  });
  render(<App />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "设置" }));
  await user.click(screen.getByRole("button", { name: "版本与更新" }));
  await user.click(screen.getByRole("button", { name: "检查更新" }));
  expect(screen.getByRole("status").textContent).toContain(
    "在线更新服务尚未接入",
  );
  expect(screen.queryByText("已是最新版")).toBeNull();
  expect(rpc).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "网络诊断" }));
  expect(screen.getByText("等待本机服务")).toBeTruthy();
  expect(
    (screen.getByRole("button", { name: "运行诊断" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
});

test("diagnostics visualizes the real report and copies only an explicit redacted summary", async () => {
  vi.mocked(rpc).mockImplementation(async (request) => {
    if (request.action === "doctor")
      return {
        control: "connected",
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
    return [] as never;
  });
  render(<App />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "网络诊断" }));
  await user.click(screen.getByRole("button", { name: "运行诊断" }));
  expect(await screen.findByText("Windows")).toBeTruthy();
  expect(screen.getByText("未找到")).toBeTruthy();
  expect(screen.getByText("0 / 1 个已启用")).toBeTruthy();
  expect(document.querySelector("pre")).toBeNull();
  await user.click(screen.getByRole("button", { name: "复制脱敏诊断" }));
  expect(copyText).toHaveBeenCalledOnce();
  const copied = vi.mocked(copyText).mock.calls[0][0];
  expect(copied).toContain("控制端：已连接");
  expect(copied).not.toMatch(/private-|10\.203|192\.0\.2|unexpected_secret/);
});

test.each(["fresh", "stale", "expired", "offline", "unlinked", "unmeasured"])(
  "diagnostic peer metrics respect actual measurements: %s",
  async (state) => {
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
        mode: state === "unlinked" ? "unknown" : "direct",
        rtt_ms: state === "unmeasured" ? undefined : 0,
        loss_percent: state === "unmeasured" ? undefined : 0,
      },
    ];
    if (state === "stale")
      status.snapshot_at = new Date(Date.now() - 60000).toISOString();
    if (state === "offline")
      vi.mocked(useService).mockReturnValue({
        status,
        error: { code: "service_unavailable", error: "服务离线" },
        refresh: vi.fn(),
        updatedAt: Date.now(),
      });
    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "网络诊断" }));
    if (state === "fresh") {
      expect(screen.getByText("0.0 ms")).toBeTruthy();
      expect(screen.getByText("0%")).toBeTruthy();
    } else {
      expect(screen.queryByText("0.0 ms")).toBeNull();
      expect(screen.queryByRole("meter")).toBeNull();
    }
    if (["stale", "expired", "offline"].includes(state))
      expect(
        (
          screen.getByRole("button", {
            name: "测量 远山 的延迟",
          }) as HTMLButtonElement
        ).disabled,
      ).toBe(true);
  },
);

test("an idle device with a zero lease does not display an expired authorization", async () => {
  status.lease_expires_at = "0001-01-01T00:00:00Z";
  render(<App />);
  await userEvent.click(screen.getByRole("button", { name: "网络诊断" }));
  expect(screen.getByText("暂无授权")).toBeTruthy();
  expect(screen.queryByText("授权已到期")).toBeNull();
});
