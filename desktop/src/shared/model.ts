export type Port = {
  protocol: "tcp" | "udp";
  port: number;
  port_end?: number;
  description?: string;
};
export type Game = {
  network: {
    version: number;
    broadcast: boolean;
    multicast: boolean;
    ethernet_types: number[];
  };
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
  owner_user_id: string;
  game: string;
  game_name: string;
  revision: number;
  capacity: number;
  expires_at: string;
  closed: boolean;
};
export type Member = {
  user_id: string;
  mac?: string;
  device_id: string;
  name: string;
  ip: string;
  last_seen: string;
};
export type Peer = {
  device_id: string;
  name: string;
  ip: string;
  mode: string;
  rtt_ms?: number;
  loss_percent?: number;
};
export type User = { id: string; name: string; kind: "guest" | "registered"; state: string; created_at: string };
export type Status = {
  user?: User;
  lan_version: number;
  lan?: {
    version: number;
    interface: string;
    mac: string;
    ipv6: string;
    mtu: number;
    ready: boolean;
  };
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
  members: Member[];
  peers: Peer[];
  ip?: string;
  snapshot_at: string;
  lease_expires_at?: string;
};
export type Management = {
  room: Room;
  game: Game;
  members: Member[];
  server_time: string;
};
export type Invitation = { code: string; expires_at: string };
export type RoomResult = { room: Room; invitation?: Invitation };
export type Action =
  | "account-login" | "account-link" | "account-poll" | "account-cancel" | "account-logout" | "account-takeover"
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
    tap_interface_present?: boolean;
    tun_device_present?: boolean;
    interface_error?: string;
    interfaces?: {
      name: string;
      up: boolean;
      mtu: number;
      addresses: string[];
    }[];
  };
};
