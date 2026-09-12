// Development-only fixtures render the real application; this entry is not a build input.
import type { Snapshot, Telemetry } from "./types";

if (!import.meta.env.DEV)
  throw new Error("Preview is available only in development");

const at = (offset = 0) => new Date(Date.now() + offset).toISOString();
const games: Snapshot["games"] = [
  {
    id: "minecraft",
    name: "Minecraft",
    summary: "和朋友一起建造新的世界。",
    source_url: "",
    cover_url: "",
    background_url: "",
    ports: [{ protocol: "tcp", port: 25565 }],
    enabled: true,
    revision: 1,
    network: {
      version: 1,
      broadcast: true,
      multicast: true,
      ethernet_types: [],
    },
  },
  {
    id: "terraria",
    name: "Terraria",
    summary: "探索、建造，与队友一起冒险。",
    source_url: "",
    cover_url: "",
    background_url: "",
    ports: [{ protocol: "tcp", port: 7777 }],
    enabled: true,
    revision: 1,
    network: {
      version: 1,
      broadcast: true,
      multicast: false,
      ethernet_types: [],
    },
  },
];
function snapshot(): Snapshot {
  return {
    games,
    nodes: [
      {
        id: "node-east",
        device_id: "infra-east",
        name: "华东 · 主节点",
        region: "上海",
        address: "east.example.com:4242",
        notes: "演示节点",
        lighthouse: true,
        relay: true,
        ip: "10.42.0.1",
        state: "active",
        generation: 1,
        revision: 3,
        last_seen: at(),
        report: {
          engine: "running",
          version: "0.3.0",
          applied_revision: 3,
          last_renewal: at(),
          lease_expires_at: at(540000),
        },
      },
      {
        id: "node-south",
        device_id: "infra-south",
        name: "华南 · 备用节点",
        region: "广州",
        address: "south.example.com:4242",
        notes: "演示节点",
        lighthouse: true,
        relay: true,
        ip: "10.42.0.2",
        state: "active",
        generation: 1,
        revision: 2,
        last_seen: at(),
        report: { engine: "running", version: "0.3.0", applied_revision: 2 },
      },
    ],
    rooms: [
      {
        id: "room-one",
        name: "周末生存计划",
        game: "minecraft",
        game_name: "Minecraft",
        owner_user_id: "user-one",
        revision: 1,
        expires_at: at(86400000),
        closed: false,
      },
      {
        id: "room-two",
        name: "一起探索新世界",
        game: "terraria",
        game_name: "Terraria",
        owner_user_id: "user-two",
        revision: 1,
        expires_at: at(43200000),
        closed: false,
      },
    ],
    operations: [],
    events: [],
    server_time: at(),
    version: "0.3.0",
    network: "10.42.0.0/16",
    deployment_id: "nodelane-room-demo",
    public_url: "https://room.example.com",
    registry: "ghcr.io/nodelane",
    ca_expires_at: at(86400000 * 180),
  };
}
function telemetry(): Telemetry {
  return {
    server_time: at(),
    retention_seconds: 60,
    stale_seconds: 15,
    geoip: true,
    series: [
      ["infra-east", "node-east", ""],
      ["infra-south", "node-south", ""],
      ["device-one", "", "room-one"],
      ["device-two", "", "room-one"],
      ["device-three", "", "room-two"],
    ].map(([device_id, node_id, room_id], index) => ({
      device_id,
      node_id,
      room_id,
      samples: Array.from({ length: 12 }, (_, i) => ({
        epoch: "demo",
        at: at((i - 11) * 5000),
        generation: 1,
        connections: index < 2 ? 4 : 1,
        traffic: {
          upload_bytes: i * 250000 + index * 50000,
          download_bytes: i * 540000 + index * 100000,
          scope: "overlay" as const,
        },
        peers: [
          {
            device_id: index === 2 ? "device-two" : "device-one",
            ip: "10.42.1.3",
            mode: "direct" as const,
            relay_ips: [],
            rtt_ms: 18 + index * 3,
            loss_percent: 0,
            probe_at: at((i - 11) * 5000),
            country: "中国",
            region: "上海",
          },
        ],
      })),
    })),
  };
}
const users = [
  {
    id: "user-one",
    name: "青山",
    kind: "registered",
    state: "active",
    created_at: at(-86400000 * 14),
  },
  {
    id: "user-two",
    name: "远山",
    kind: "guest",
    state: "active",
    created_at: at(-86400000 * 3),
  },
];
const originalFetch = window.fetch.bind(window);
window.fetch = async (input, init) => {
  const path = new URL(
    typeof input === "string"
      ? input
      : input instanceof URL
        ? input.href
        : input.url,
    location.origin,
  ).pathname;
  if (!path.startsWith("/v2/admin/")) return originalFetch(input, init);
  if (init?.method && !["GET", "HEAD"].includes(init.method))
    return Response.json(
      {
        contract: "interaction-1",
        request_id: "preview",
        code: "input_invalid",
        details: {},
      },
      { status: 400 },
    );
  if (path.endsWith("/events")) {
    let timer: ReturnType<typeof setInterval>;
    const stream = new ReadableStream({
      start(controller) {
        const publish = () =>
          controller.enqueue(
            new TextEncoder().encode(
              `event: snapshot\ndata: ${JSON.stringify(snapshot())}\n\n`,
            ),
          );
        publish();
        timer = setInterval(publish, 5000);
        init?.signal?.addEventListener(
          "abort",
          () => {
            clearInterval(timer);
            controller.close();
          },
          { once: true },
        );
      },
      cancel() {
        clearInterval(timer);
      },
    });
    return new Response(stream, {
      headers: { "Content-Type": "text/event-stream" },
    });
  }
  let data: unknown = {};
  if (path.endsWith("/setup")) data = { initialized: true, configured: true };
  else if (path.endsWith("/session"))
    data = { username: "演示管理员", csrf: "preview-only" };
  else if (path.endsWith("/snapshot")) data = snapshot();
  else if (path.endsWith("/telemetry")) data = telemetry();
  else if (path.endsWith("/users")) data = { users, next: "" };
  else if (path.includes("/users/"))
    data = {
      user: users.find((u) => path.endsWith(u.id)) || users[0],
      devices: [],
      rooms: snapshot().rooms,
      sessions: [],
    };
  else if (path.endsWith("/oidc"))
    data = {
      revision: 1,
      issuer: "https://auth.example.com/oidc",
      client_id: "nodelane-room",
      enabled: true,
    };
  else if (path.endsWith("/updates"))
    data = {
      sources: [],
      releases: [],
      policies: [],
      devices: [],
      versions: [],
      attempts: [],
      repository_revision: 0,
    };
  else if (path.includes("/rooms/"))
    data = {
      room:
        snapshot().rooms.find((r) => path.endsWith(r.id)) ||
        snapshot().rooms[0],
      members: [
        {
          user_id: "user-one",
          device_id: "device-one",
          name: "青山",
          ip: "10.42.1.2",
          last_seen: at(),
        },
        {
          user_id: "user-two",
          device_id: "device-two",
          name: "远山",
          ip: "10.42.1.3",
          last_seen: at(),
        },
      ],
      game: games[0],
    };
  return Response.json({
    contract: "interaction-1",
    request_id: "preview",
    code: "ok",
    data,
  });
};
void import("./main");
