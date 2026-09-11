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
  measured_at?: string;
};
export type User = {
  id: string;
  name: string;
  kind: "guest" | "registered";
  state: string;
  created_at: string;
};
export type Status = {
  service_instance_id: string;
  status_seq: number;
  service: string;
  identity: string;
  operation: string;
  freshness: { observed_at: string; snapshot_at: string };
  membership: {
    device_id?: string;
    valid_until?: string;
    state: string;
    reason?: string;
    room_id?: string;
    revision: number;
  };
  permissions: { manage: boolean; join: boolean; leave: boolean };
  network: {
    state: string;
    reason?: string;
    generation: number;
    applied_game_revision: number;
  };
  pending_operations: Operation[];
  issues: {
    code: string;
    scope: string;
    occurred_at: string;
    resolved_at?: string;
  }[];
  update?: UpdateStatus;
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
  permissions: Status["permissions"];
  room: Room;
  game: Game;
  members: Member[];
  server_time: string;
};
export type Invitation = { code: string; expires_at: string; revision: number };
export type InviteInfo = {
  active: boolean;
  revision: number;
  expires_at?: string;
};
export type Envelope<T> = {
  contract: string;
  code: string;
  message: string;
  origin: string;
  request_id: string;
  operation_id?: string;
  data: T;
  control_http_status?: number | null;
  cause_request_id?: string;
  details?: Record<string, unknown>;
  retry?: { kind: string; after_ms?: number };
};
export type Operation = {
  id: string;
  action?: string;
  state: string;
  known_commit: boolean;
  deadline: string;
  result?: Envelope<unknown>;
};
export type RoomResult = { room: Room; invitation?: Invitation };
export type Action =
  | "capabilities"
  | "get-operation"
  | "network-stop"
  | "network-retry"
  | "invite-info"
  | "invite-revoke"
  | "owner-join"
  | "account-devices"
  | "account-status"
  | "revoke-device"
  | "update-check"
  | "update-status"
  | "update-install"
  | "account-login"
  | "account-link"
  | "account-poll"
  | "account-cancel"
  | "account-logout"
  | "account-takeover"
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
  command_id?: string;
  action: Action;
  room?: string;
  server?: string;
  name?: string;
  target?: string;
  body?: unknown;
};
export type Failure = {
  code: string;
  error: string;
  request_id?: string;
  operation_id?: string;
  origin?: string;
  control_http_status?: number | null;
  cause_request_id?: string;
  details?: Record<string, unknown>;
  retry?: { kind: string; after_ms?: number };
};
export type UpdateStatus = {
  state: string;
  error_code?: string;
  downloaded: number;
  checked_at: string;
  required: boolean;
  policy?: { minimum_version?: string; effective_at?: string };
  release?: { id: string; version: string; notes: string; size: number };
};
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
