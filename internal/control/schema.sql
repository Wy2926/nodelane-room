CREATE TABLE IF NOT EXISTS schema_version (version integer PRIMARY KEY);
CREATE TABLE IF NOT EXISTS settings (key text PRIMARY KEY, value text NOT NULL);
CREATE TABLE IF NOT EXISTS devices (
 id text PRIMARY KEY, name text NOT NULL, public_key bytea NOT NULL UNIQUE,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS challenges (
 id text PRIMARY KEY, device_id text NOT NULL, name text NOT NULL, public_key bytea NOT NULL,
 nonce bytea NOT NULL, scope text NOT NULL DEFAULT 'player', grant_id text, expires_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
 token_hash text PRIMARY KEY, device_id text NOT NULL REFERENCES devices(id), scope text NOT NULL DEFAULT 'player', expires_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS rooms (
 id text PRIMARY KEY, name text NOT NULL, owner_id text NOT NULL REFERENCES devices(id), game text NOT NULL,
 capacity integer NOT NULL CHECK (capacity BETWEEN 1 AND 32), revision bigint NOT NULL DEFAULT 1,
 expires_at timestamptz NOT NULL, closed boolean NOT NULL DEFAULT false
);
CREATE TABLE IF NOT EXISTS members (
 room_id text NOT NULL REFERENCES rooms(id), device_id text NOT NULL REFERENCES devices(id),
 ip text NOT NULL, active boolean NOT NULL DEFAULT true, banned boolean NOT NULL DEFAULT false,
 last_seen timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(room_id,device_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS one_active_room ON members(device_id) WHERE active;
CREATE UNIQUE INDEX IF NOT EXISTS unique_active_ip ON members(ip) WHERE active;
CREATE TABLE IF NOT EXISTS addresses (ip text PRIMARY KEY, holder text NOT NULL UNIQUE, release_after timestamptz);
CREATE TABLE IF NOT EXISTS invitations (
 code_hash text PRIMARY KEY, room_id text NOT NULL REFERENCES rooms(id), expires_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS nodes (
 id text PRIMARY KEY, name text NOT NULL, region text NOT NULL, address text NOT NULL,
 lighthouse boolean NOT NULL, relay boolean NOT NULL, notes text NOT NULL DEFAULT '',
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','active','draining','disabled','revoked')),
 generation bigint NOT NULL DEFAULT 1, revision bigint NOT NULL DEFAULT 1,
 last_seen timestamptz NOT NULL DEFAULT 'epoch', report jsonb NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS node_configs (node_id text NOT NULL REFERENCES nodes(id), revision bigint NOT NULL, config jsonb NOT NULL, PRIMARY KEY(node_id,revision));
CREATE TABLE IF NOT EXISTS node_bindings (
 device_id text PRIMARY KEY REFERENCES devices(id), node_id text NOT NULL REFERENCES nodes(id),
 generation bigint NOT NULL, ip text NOT NULL, revoked_at timestamptz, UNIQUE(node_id,generation)
);
CREATE UNIQUE INDEX IF NOT EXISTS node_current_binding ON node_bindings(node_id) WHERE revoked_at IS NULL;
CREATE TABLE IF NOT EXISTS enrollment_keys (
 id text PRIMARY KEY, key_hash text NOT NULL UNIQUE, node_id text NOT NULL REFERENCES nodes(id),
 generation bigint NOT NULL, revision bigint NOT NULL, expires_at timestamptz NOT NULL,
 consumed_by text, revoked boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS node_operations (
 id text PRIMARY KEY, node_id text NOT NULL REFERENCES nodes(id), generation bigint NOT NULL,
 revision bigint NOT NULL, action text NOT NULL, state text NOT NULL DEFAULT 'pending',
 error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS administrator (
 id integer PRIMARY KEY CHECK(id=1), username text NOT NULL, password_hash text NOT NULL
);
CREATE TABLE IF NOT EXISTS admin_bootstrap (
 id integer PRIMARY KEY CHECK(id=1), token_hash text NOT NULL, expires_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS admin_sessions (
 token_hash text PRIMARY KEY, csrf_hash text NOT NULL, expires_at timestamptz NOT NULL,
 last_seen timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS admin_events (
 id bigserial PRIMARY KEY, actor text NOT NULL, kind text NOT NULL, target text NOT NULL,
 detail jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS certificates (
 fingerprint text PRIMARY KEY, device_id text NOT NULL REFERENCES devices(id), room_id text,
 ip text NOT NULL, public_key bytea NOT NULL, expires_at timestamptz NOT NULL, revoked boolean NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS certificates_device ON certificates(device_id,expires_at);
CREATE TABLE IF NOT EXISTS endpoints (
 id text PRIMARY KEY, room_id text NOT NULL REFERENCES rooms(id), device_id text NOT NULL REFERENCES devices(id),
 protocol text NOT NULL CHECK (protocol IN ('tcp','udp')), port integer NOT NULL CHECK (port BETWEEN 1 AND 65535),
 motd text NOT NULL DEFAULT '', expires_at timestamptz NOT NULL,
 UNIQUE(room_id,device_id,protocol,port)
);
CREATE TABLE IF NOT EXISTS events (
 room_id text NOT NULL REFERENCES rooms(id), revision bigint NOT NULL, kind text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(room_id,revision)
);
CREATE TABLE IF NOT EXISTS idempotency (
 device_id text NOT NULL, key text NOT NULL, request_hash text NOT NULL, response jsonb NOT NULL,
 expires_at timestamptz NOT NULL, PRIMARY KEY(device_id,key)
);
CREATE TABLE IF NOT EXISTS rate_limits (key text PRIMARY KEY, count integer NOT NULL, window_start timestamptz NOT NULL);
INSERT INTO schema_version(version) VALUES (2) ON CONFLICT DO NOTHING;
