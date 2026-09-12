// Development-only UI fixture. This entry is not part of the desktop build.
import { createRoot } from "react-dom/client";
import { mockIPC, mockWindows } from "@tauri-apps/api/mocks";
import { clientVersion } from "./native/api";
import { App } from "./app/App";
import type { Game, Request, Room, Status } from "./shared/model";
import "./styles/index.css";
import { t, useLanguage, setLanguage, languageStorageKey } from "./i18n";

if (!import.meta.env.DEV)
  throw new Error("UI preview is only available in development");
const scenario = new URLSearchParams(location.search).get("state");
if (scenario === "language") localStorage.removeItem(languageStorageKey);
else
  setLanguage(
    new URLSearchParams(location.search).get("lang") === "en-US"
      ? "en-US"
      : "zh-CN",
  );
const games: Game[] = [
  {
    id: "custom",
    name: "通用房间",
    summary: "和朋友一起玩。",
    network: {
      version: 1,
      broadcast: true,
      multicast: true,
      ethernet_types: [],
    },
    enabled: true,
    revision: 1,
    source_url: "",
    cover_url: "",
    background_url: "",
    ports: [
      { protocol: "tcp", port: 1, port_end: 65535 },
      { protocol: "udp", port: 1, port_end: 65535 },
    ],
  },
];
const owned = [
  makeRoom("周末的冒险小队", games[0]),
  makeRoom("今晚一起玩", games[0]),
];
let game = games[0];
let room: Room | undefined = ["room", "metrics"].includes(scenario || "")
  ? owned[0]
  : undefined;
let connected = !!room;
let initialized = !["setup", "login", "language"].includes(scenario || "");
let updateState = scenario === "update" ? "available" : "idle";
let updateStarted = 0;
let autoStart = false;
let playerName = "旅人";
let loginState = scenario === "login" ? "waiting" : "none";
function makeRoom(name: string, selected: Game): Room {
  return {
    id: crypto.randomUUID(),
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
  const measured = scenario === "metrics" && connected;
  return {
    room_creation:
      scenario === "restricted"
        ? { allowed: false, reason: "account_disabled" }
        : { allowed: true },
    user:
      initialized && scenario !== "signed-out"
        ? {
            id: "preview-user",
            name: playerName,
            kind: scenario === "account" ? "registered" : "guest",
            state: scenario === "restricted" ? "disabled" : "active",
            created_at: new Date().toISOString(),
          }
        : undefined,
    service_instance_id: "preview",
    status_seq: ++statusSequence,
    service: "ready",
    identity:
      scenario === "signed-out"
        ? "signed_out"
        : initialized
          ? "active"
          : "unconfigured",
    operation: "idle",
    membership: {
      state: connected ? "active" : "none",
      room_id: room?.id,
      device_id: "preview-player",
      revision: room?.revision || 0,
      valid_until: measured
        ? new Date(Date.now() + 60000).toISOString()
        : undefined,
    },
    permissions: {
      manage: !!room,
      join: !!room && !connected,
      leave: connected,
    },
    network: {
      state: measured ? "ready" : connected ? "preparing" : "stopped",
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
    name: playerName,
    device_id: initialized ? "preview-player" : "",
    control: "online",
    engine: measured ? "running" : "stopped",
    lease_expires_at: measured
      ? new Date(Date.now() + 60000).toISOString()
      : undefined,
    ip: measured ? "10.203.0.2" : undefined,
    lan: measured
      ? {
          ready: true,
          version: 1,
          ipv6: "fd00::2",
          interface: "NodeLane",
          mtu: 1300,
          mac: "02:00:00:00:00:02",
        }
      : undefined,
    selected_room: connected ? room!.id : "",
    room: connected ? room : undefined,
    game: connected ? game : undefined,
    members: connected
      ? [
          {
            device_id: "preview-player",
            user_id: "preview-user",
            name: playerName,
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
            mode: measured ? "direct" : "unknown",
            measured_at: measured ? new Date().toISOString() : undefined,
            rtt_ms: measured ? 18.4 : undefined,
            loss_percent: measured ? 0 : undefined,
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
      if (scenario === "error")
        throw { code: "local_service_unavailable", error: "预览服务故障" };
      switch (request.action) {
        case "account-poll":
          return { state: loginState };
        case "status":
          return status();
        case "init":
          initialized = true;
          playerName = request.name || playerName;
          return {};
        case "games":
          return games;
        case "rooms":
          return { rooms: initialized ? owned : [], truncated: false };
        case "capabilities":
          return { oidc_enabled: true, ready: true };
        case "manage": {
          const managed = owned.find((r) => r.id === request.room) || room;
          return {
            permissions: { manage: true, join: !connected, leave: connected },
            room: managed,
            game: games.find((g) => g.id === managed?.game),
            members: status().members,
            server_time: new Date().toISOString(),
          };
        }
        case "owner-join":
          room = owned.find((r) => r.id === request.room);
          game = games.find((g) => g.id === room?.game) || games[0];
          connected = !!room;
          return { room };
        case "create": {
          if (scenario === "restricted") throw { code: "account_disabled" };
          const body = request.body as { name: string; game: string };
          game = games.find((g) => g.id === body.game) || games[0];
          room = makeRoom(body.name, game);
          owned.push(room);
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
          room.owner_user_id = "preview-friend";
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
          owned.splice(
            owned.findIndex((r) => r.id === request.room),
            1,
          );
          connected = false;
          room = undefined;
          return {};
        case "account-status":
          return { user: status().user, room_creation: status().room_creation };
        case "account-devices":
          return [
            { device_id: "preview-player", name: "我的电脑", revoked: false },
            { device_id: "preview-laptop", name: "游戏笔记本", revoked: false },
            { device_id: "preview-old", name: "旧电脑", revoked: true },
          ];
        case "account-login":
        case "account-link":
          loginState = "waiting";
          return {};
        case "account-cancel":
          loginState = "none";
          return {};
        case "update-status":
        case "update-check":
          if (updateState === "downloading" && Date.now() - updateStarted >= 15000) updateState = "ready";
          return { state: updateState, required: false, downloaded: updateState === "downloading" ? Math.min(125829120, Math.floor((Date.now() - updateStarted) / 15000 * 125829120)) : 0,
            release: scenario === "update" ? { id: "preview-release", version: "0.4.0", size: 125829120, notes: "改进房间连接体验\n优化网络稳定性，修复已知问题。" } : undefined };
        case "update-download":
          updateState = "downloading";
          updateStarted = Date.now();
          return {};
        case "update-cancel":
          updateState = "available";
          return {};
        case "update-install":
          updateState = "installing";
          return {};
        case "network-stop":
        case "network-retry":
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
