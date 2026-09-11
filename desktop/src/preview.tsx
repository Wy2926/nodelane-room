// Development-only UI fixture. This entry is not part of the desktop build.
import { createRoot } from "react-dom/client";
import { mockIPC, mockWindows } from "@tauri-apps/api/mocks";
import { clientVersion } from "./native/api";
import { App } from "./app/App";
import type { Game, Request, Room, Status } from "./shared/model";
import "./styles/index.css";
import { t, useLanguage } from "./i18n";

if (!import.meta.env.DEV)
  throw new Error("UI preview is only available in development");
const scenario = new URLSearchParams(location.search).get("state");
const games: Game[] = [
  [
    "892970",
    "英灵神殿",
    "穿过迷雾，驶向未知的海岸。与朋友建造家园，探索属于你们的北欧世界。",
  ],
  [
    "413150",
    "星露谷物语",
    "把平凡的日子过成喜欢的样子。耕种、垂钓，与朋友分享农场的每一个季节。",
  ],
  [
    "105600",
    "泰拉瑞亚",
    "挖掘，战斗，探索，建造。和朋友一起发现地下世界的无限可能。",
  ],
  [
    "322330",
    "饥荒联机版",
    "夜幕将至，篝火已燃起。与伙伴一起，在奇妙而危险的荒野中生存。",
  ],
  ["108600", "僵尸毁灭工程", "集结伙伴，寻找物资，建立你们的生存据点。"],
  ["custom", "通用游戏", "为你喜欢的游戏配置端口，开启一个属于你们的世界。"],
].map(([id, name, summary]) => ({
  network: { version: 1, broadcast: true, multicast: true, ethernet_types: [] },
  id,
  name,
  summary,
  enabled: true,
  revision: 1,
  source_url: "",
  cover_url: id === "custom" ? "" : "preview",
  background_url: id === "custom" ? "" : "preview",
  ports: id === "custom" ? [] : [{ protocol: "udp", port: 2456 }],
}));
let game = games[0];
let room: Room | undefined =
  scenario === "room" ? makeRoom("雾林小屋 · 今晚继续冒险", game) : undefined;
let connected = !!room;
let initialized = scenario !== "setup";
let autoStart = false;
function makeRoom(name: string, selected: Game): Room {
  return {
    id: "preview-room",
    name,
    owner_user_id: "preview-user",
    game: selected.id,
    game_name: selected.name,
    revision: 1,
    capacity: 4,
    closed: false,
    expires_at: new Date(Date.now() + 3600000).toISOString(),
  };
}
let statusSequence = 0;
function status(): Status {
  return {
    user: initialized
      ? {
          id: "preview-user",
          name: "旅人",
          kind: "guest",
          state: "active",
          created_at: new Date().toISOString(),
        }
      : undefined,
    service_instance_id: "preview",
    status_seq: ++statusSequence,
    service: "ready",
    identity: initialized ? "active" : "unconfigured",
    operation: "idle",
    membership: {
      state: connected ? "active" : "none",
      room_id: room?.id,
      device_id: "preview-player",
      revision: room?.revision || 0,
    },
    permissions: {
      manage: !!room,
      join: !!room && !connected,
      leave: connected,
    },
    network: {
      state: connected ? "preparing" : "stopped",
      generation: 0,
      applied_game_revision: 0,
    },
    freshness: {
      observed_at: new Date().toISOString(),
      snapshot_at: new Date().toISOString(),
    },
    issues: [],
    pending_operations: [],
    version: clientVersion,
    protocol_version: 3,
    lan_version: 1,
    server: "https://room.nodelane.net",
    name: "旅人",
    device_id: initialized ? "preview-player" : "",
    control: "online",
    engine: "stopped",
    selected_room: connected ? room!.id : "",
    room: connected ? room : undefined,
    game: connected ? game : undefined,
    members: connected
      ? [
          {
            device_id: "preview-player",
            user_id: "preview-user",
            name: "旅人",
            ip: "10.203.0.2",
            last_seen: new Date().toISOString(),
          },
          {
            device_id: "preview-friend",
            user_id: "preview-friend",
            name: "远山",
            ip: "10.203.0.3",
            last_seen: new Date().toISOString(),
          },
        ]
      : [],
    peers: connected
      ? [
          {
            device_id: "preview-friend",
            name: "远山",
            ip: "10.203.0.3",
            mode: "unknown",
          },
        ]
      : [],
    snapshot_at: new Date().toISOString(),
  };
}
Object.defineProperty(window, "isTauri", { value: true, configurable: true });
mockWindows("main");
mockIPC(
  async (command, args) => {
    const payload = args as Record<string, unknown> | undefined;
    if (command === "plugin:window|is_maximized") return false;
    if (command.startsWith("plugin:window|"))
      throw { code: "preview", error: "窗口控制仅在桌面应用中生效。" };
    if (command === "game_image")
      return `https://shared.fastly.steamstatic.com/store_item_assets/steam/apps/${payload?.game}/${payload?.kind === "background" ? "library_hero.jpg" : "library_600x900.jpg"}`;
    if (command === "plugin:autostart|is_enabled") return autoStart;
    if (command === "plugin:autostart|enable") {
      autoStart = true;
      return;
    }
    if (command === "plugin:autostart|disable") {
      autoStart = false;
      return;
    }
    if (command === "plugin:clipboard-manager|write_text") {
      await navigator.clipboard.writeText(String(payload?.text || ""));
      return;
    }
    if (command === "notify_state" || command === "set_language") return;
    if (command === "exit_app")
      throw { code: "preview", error: "界面预览中不会退出桌面应用。" };
    const request = payload?.requestData as Request;
    if (!request) throw new Error("Unsupported preview command");
    const execute = async () => {
      switch (request.action) {
        case "account-poll":
          return { state: "none" };
        case "status":
          if (scenario === "error")
            throw { code: "service_unavailable", error: "预览服务故障" };
          return status();
        case "init":
          initialized = true;
          return {};
        case "games":
          return games;
        case "rooms":
          return { rooms: room ? [room] : [], truncated: false };
        case "capabilities":
          return { oidc_enabled: true, ready: true };
        case "manage":
          return {
            permissions: { manage: true, join: false, leave: true },
            room,
            game,
            members: status().members,
            server_time: new Date().toISOString(),
          };
        case "create": {
          const body = request.body as { name: string; game: string };
          game = games.find((g) => g.id === body.game) || games[0];
          room = makeRoom(body.name, game);
          connected = true;
          return {
            room,
            invitation: {
              code: "UI-PREVIEW-ONLY",
              revision: room?.revision || 1,
              expires_at: new Date(Date.now() + 600000).toISOString(),
            },
          };
        }
        case "join":
          room = makeRoom("朋友的房间", game);
          connected = true;
          return { room };
        case "invite-info":
          return {
            active: !!room,
            revision: room?.revision || 1,
            expires_at: new Date(Date.now() + 600000).toISOString(),
          };
        case "invite":
          return {
            code: "UI-PREVIEW-ONLY",
            revision: room?.revision || 1,
            expires_at: new Date(Date.now() + 600000).toISOString(),
          };
        case "leave":
          connected = false;
          return {};
        case "close":
          connected = false;
          room = undefined;
          return {};
        case "doctor":
          return {
            nebula_version: "1.11.1",
            control: "online",
            engine: "stopped",
            platform: {
              os: "windows",
              arch: "amd64",
              tap_interface_present: true,
              interfaces: [
                {
                  name: "示例以太网",
                  up: true,
                  mtu: 1500,
                  addresses: ["192.0.2.10/24"],
                },
                { name: "示例无线网卡", up: false, mtu: 1500, addresses: [] },
              ],
            },
          };
        default:
          throw {
            code: "preview",
            error: "此操作需要真实桌面服务；预览中未执行。",
          };
      }
    };
    return {
      contract: "interaction-1",
      code: "ok",
      message: "",
      origin: "service",
      request_id: crypto.randomUUID(),
      operation_id: request.command_id,
      data: await execute(),
      details: {},
      retry: { kind: "none" },
    };
  },
  { shouldMockEvents: true },
);

function PreviewBadge() {
  useLanguage();
  return <div className="preview-badge">{t("preview.badge")}</div>;
}

const root = createRoot(document.getElementById("root")!);
if (import.meta.hot) import.meta.hot.dispose(() => root.unmount());
root.render(
  <>
    <App />
    <PreviewBadge />
  </>,
);
