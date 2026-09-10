export interface Room {
  id: string;
  name: string;
  game: string;
  owner_id: string;
  expires_at: string;
  closed: boolean;
}
export interface NodeConfig {
  name: string;
  region: string;
  address: string;
  notes: string;
  lighthouse: boolean;
  relay: boolean;
}
export interface InfraNode extends NodeConfig {
  id: string;
  device_id: string;
  ip: string;
  state: string;
  generation: number;
  revision: number;
  last_seen: string;
  report: {
    engine?: string;
    error?: string;
    version?: string;
    applied_revision?: number;
    last_renewal?: string;
    lease_expires_at?: string;
  };
}
export interface Operation {
  id: string;
  node_id: string;
  action: string;
  state: string;
  error?: string;
  created_at: string;
}
export interface Audit {
  id: number;
  actor: string;
  kind: string;
  target: string;
  detail: unknown;
  created_at: string;
}
export interface Snapshot {
  nodes: InfraNode[];
  rooms: Room[];
  operations: Operation[];
  events: Audit[];
  server_time: string;
  version: string;
  network: string;
  deployment_id: string;
  public_url: string;
  registry: string;
  ca_expires_at: string;
}
export interface RoomSnapshot {
  room: Room;
  members: { device_id: string; name: string; ip: string; last_seen: string }[];
  endpoints: {
    id: string;
    device_id: string;
    protocol: string;
    port: number;
    expires_at: string;
  }[];
}
export interface Link {
  device_id: string;
  ip: string;
  mode: "direct" | "relay" | "unknown";
  remote?: string;
  relay_ips: string[];
  rtt_ms?: number;
  loss_percent?: number;
  probe_at: string;
  country?: string;
  region?: string;
}
export interface Sample {
  epoch: string;
  at: string;
  generation: number;
  connections: number;
  peers: Link[] | null;
  traffic?: {
    upload_bytes: number;
    download_bytes: number;
    scope: "overlay" | "nebula_udp";
  };
}
export interface Series {
  device_id: string;
  room_id?: string;
  node_id?: string;
  samples: Sample[];
}
export interface Telemetry {
  server_time: string;
  retention_seconds: number;
  stale_seconds: number;
  geoip: boolean;
  geoip_provider?: string;
  series: Series[];
}
export interface Session {
  username: string;
  csrf: string;
}
export type API = <T>(
  path: string,
  body?: unknown,
  method?: string,
  signal?: AbortSignal,
) => Promise<T>;
