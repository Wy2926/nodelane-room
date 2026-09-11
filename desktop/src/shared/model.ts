export type Port = {
  protocol: "tcp" | "udp";
  port: number;
  port_end?: number;
  description?: string;
};
export type Game = {
  id: string;
  name: string;
  summary: string;
  cover_url: string;
  background_url: string;
  source_url: string;
  ports: Port[];
  enabled: boolean;
  revision: number;
};
export type Room = {
  id: string;
  name: string;
  owner_id: string;
  game: string;
  game_name: string;
  revision: number;
  capacity: number;
  expires_at: string;
  closed: boolean;
};
export type Member = {
  device_id: string;
  name: string;
  ip: string;
  last_seen: string;
};
export type Endpoint = Port & {
  id: string;
  device_id: string;
  expires_at: string;
};
export type Peer = {
  device_id: string;
  name: string;
  ip: string;
  mode: string;
  rtt_ms?: number;
  loss_percent?: number;
};
export type Status = {
  version: string;
  protocol_version: number;
  server: string;
  name: string;
  device_id: string;
  control: string;
  engine: string;
  error?: string;
  selected_room: string;
  room?: Room;
  game?: Game;
  ports: Port[] | null;
  members: Member[];
  endpoints: Endpoint[];
  peers: Peer[];
  ip?: string;
  snapshot_at: string;
  lease_expires_at?: string;
};
export type Management = {
  room: Room;
  game: Game;
  members: Member[];
  endpoints: Endpoint[];
  server_time: string;
};
export type Invitation = { code: string; expires_at: string };
export type RoomResult = { room: Room; invitation?: Invitation };
export type Action =
  | "status"
  | "init"
  | "games"
  | "rooms"
  | "manage"
  | "members"
  | "create"
  | "join"
  | "invite"
  | "kick"
  | "transfer"
  | "leave"
  | "close"
  | "port"
  | "remove-port"
  | "ping"
  | "doctor";
export type Request = {
  action: Action;
  room?: string;
  server?: string;
  name?: string;
  target?: string;
  body?: unknown;
};
export type Failure = { code: string; error: string };
export type Diagnostic = {
  nebula_version: string;
  control: string;
  engine: string;
  error?: string;
  lease_expires_at?: string;
  platform: {
    os: string;
    arch: string;
    wintun_present?: boolean;
    tun_device_present?: boolean;
    interface_error?: string;
    interfaces?: { name: string; up: boolean; mtu: number; addresses: string[] }[];
  };
};
