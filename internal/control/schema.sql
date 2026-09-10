CREATE TABLE schema_version (version integer PRIMARY KEY);
CREATE TABLE settings (key text PRIMARY KEY, value text NOT NULL);
CREATE TABLE deployment (
 id integer PRIMARY KEY CHECK(id=1), database_url text NOT NULL, public_url text NOT NULL,
 registry text NOT NULL, ca_cert text NOT NULL, ca_key text NOT NULL
);
CREATE TABLE devices (
 id text PRIMARY KEY, name text NOT NULL, public_key bytea NOT NULL UNIQUE,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE challenges (
 id text PRIMARY KEY, device_id text NOT NULL, name text NOT NULL, public_key bytea NOT NULL,
 nonce bytea NOT NULL, scope text NOT NULL DEFAULT 'player', grant_id text, expires_at timestamptz NOT NULL
);
CREATE TABLE sessions (
 token_hash text PRIMARY KEY, device_id text NOT NULL REFERENCES devices(id), scope text NOT NULL DEFAULT 'player', expires_at timestamptz NOT NULL
);
CREATE TABLE games (
 id text PRIMARY KEY, name text NOT NULL, summary text NOT NULL DEFAULT '', source_url text NOT NULL DEFAULT '',
 ports jsonb NOT NULL DEFAULT '[]', enabled boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1
);
CREATE TABLE game_images (
 game_id text NOT NULL REFERENCES games(id), kind text NOT NULL CHECK(kind IN ('cover','background')),
 data bytea NOT NULL CHECK(octet_length(data) BETWEEN 1 AND 5242880),
 content_type text NOT NULL CHECK(content_type IN ('image/jpeg','image/png')), PRIMARY KEY(game_id,kind)
);
INSERT INTO games(id,name,summary,ports,enabled) VALUES
 ('custom','通用游戏','自行登记需要开放的 TCP/UDP 端口。','[]',true),
 ('minecraft-java','Minecraft Java','通过虚拟 IP 和服务端配置端口直接连接。','[{"protocol":"tcp","port":25565}]',true);
CREATE TABLE rooms (
 id text PRIMARY KEY, name text NOT NULL, owner_id text NOT NULL REFERENCES devices(id), game text NOT NULL REFERENCES games(id),
 capacity integer NOT NULL CHECK (capacity BETWEEN 1 AND 32), revision bigint NOT NULL DEFAULT 1,
 expires_at timestamptz NOT NULL, closed boolean NOT NULL DEFAULT false
);
CREATE TABLE members (
 room_id text NOT NULL REFERENCES rooms(id), device_id text NOT NULL REFERENCES devices(id),
 ip text NOT NULL, active boolean NOT NULL DEFAULT true, banned boolean NOT NULL DEFAULT false,
 last_seen timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(room_id,device_id)
);
CREATE UNIQUE INDEX one_active_room ON members(device_id) WHERE active;
CREATE UNIQUE INDEX unique_active_ip ON members(ip) WHERE active;
CREATE TABLE addresses (ip text PRIMARY KEY, holder text NOT NULL UNIQUE, release_after timestamptz);
CREATE TABLE invitations (
 code_hash text PRIMARY KEY, room_id text NOT NULL REFERENCES rooms(id), expires_at timestamptz NOT NULL
);
CREATE TABLE nodes (
 id text PRIMARY KEY, name text NOT NULL, region text NOT NULL, address text NOT NULL,
 lighthouse boolean NOT NULL, relay boolean NOT NULL, notes text NOT NULL DEFAULT '',
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','active','draining','disabled','revoked')),
 generation bigint NOT NULL DEFAULT 1, revision bigint NOT NULL DEFAULT 1,
 last_seen timestamptz NOT NULL DEFAULT 'epoch', report jsonb NOT NULL DEFAULT '{}'
);
CREATE TABLE node_configs (node_id text NOT NULL REFERENCES nodes(id), revision bigint NOT NULL, config jsonb NOT NULL, PRIMARY KEY(node_id,revision));
CREATE TABLE node_bindings (
 device_id text PRIMARY KEY REFERENCES devices(id), node_id text NOT NULL REFERENCES nodes(id),
 generation bigint NOT NULL, ip text NOT NULL, revoked_at timestamptz, UNIQUE(node_id,generation)
);
CREATE UNIQUE INDEX node_current_binding ON node_bindings(node_id) WHERE revoked_at IS NULL;
CREATE TABLE enrollment_keys (
 id text PRIMARY KEY, key_hash text NOT NULL UNIQUE, node_id text NOT NULL REFERENCES nodes(id),
 generation bigint NOT NULL, revision bigint NOT NULL, expires_at timestamptz NOT NULL,
 consumed_by text, revoked boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE node_operations (
 id text PRIMARY KEY, node_id text NOT NULL REFERENCES nodes(id), generation bigint NOT NULL,
 revision bigint NOT NULL, action text NOT NULL, state text NOT NULL DEFAULT 'pending',
 error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL
);
CREATE TABLE administrator (
 id integer PRIMARY KEY CHECK(id=1), username text NOT NULL, password_hash text NOT NULL
);
CREATE TABLE admin_sessions (
 token_hash text PRIMARY KEY, csrf_hash text NOT NULL, expires_at timestamptz NOT NULL,
 last_seen timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE admin_events (
 id bigserial PRIMARY KEY, actor text NOT NULL, kind text NOT NULL, target text NOT NULL,
 detail jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE certificates (
 fingerprint text PRIMARY KEY, device_id text NOT NULL REFERENCES devices(id), room_id text,
 ip text NOT NULL, public_key bytea NOT NULL, expires_at timestamptz NOT NULL, revoked boolean NOT NULL DEFAULT false
);
CREATE INDEX certificates_device ON certificates(device_id,expires_at);
CREATE TABLE endpoints (
 id text PRIMARY KEY, room_id text NOT NULL REFERENCES rooms(id), device_id text NOT NULL REFERENCES devices(id),
 protocol text NOT NULL CHECK (protocol IN ('tcp','udp')), port integer NOT NULL CHECK (port BETWEEN 1 AND 65535),
 expires_at timestamptz NOT NULL,
 UNIQUE(room_id,device_id,protocol,port)
);
CREATE TABLE events (
 room_id text NOT NULL REFERENCES rooms(id), revision bigint NOT NULL, kind text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(room_id,revision)
);
CREATE TABLE idempotency (
 device_id text NOT NULL, key text NOT NULL, request_hash text NOT NULL, response jsonb NOT NULL,
 expires_at timestamptz NOT NULL, PRIMARY KEY(device_id,key)
);
CREATE TABLE rate_limits (key text PRIMARY KEY, count integer NOT NULL, window_start timestamptz NOT NULL);
INSERT INTO schema_version(version) VALUES (3);
