-- Discurd ScyllaDB schema. Applied automatically by the scylla-init container.
-- Local dev: SimpleStrategy RF=1. For the 3-node cluster see docs/DEPLOYMENT.md
-- (ALTER KEYSPACE to NetworkTopologyStrategy RF=3, then repair).

CREATE KEYSPACE IF NOT EXISTS discurd
  WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

USE discurd;

CREATE TYPE IF NOT EXISTS attachment (
  url          text,
  filename     text,
  size         bigint,
  content_type text
);

-- ---------- users ----------
CREATE TABLE IF NOT EXISTS users (
  user_id       uuid PRIMARY KEY,
  username      text,
  email         text,
  password_hash text,
  avatar_url    text,
  bio           text,
  accent_color  text,
  pronouns      text,
  created_at    timestamp
);

-- Uniqueness + login lookups. Written with LWT (IF NOT EXISTS) before users.
CREATE TABLE IF NOT EXISTS users_by_email (
  email   text PRIMARY KEY,
  user_id uuid
);

CREATE TABLE IF NOT EXISTS users_by_username (
  username text PRIMARY KEY,
  user_id  uuid
);

-- ---------- guilds ----------
CREATE TABLE IF NOT EXISTS guilds (
  guild_id   uuid PRIMARY KEY,
  name       text,
  owner_id   uuid,
  icon_url   text,
  created_at timestamp
);

CREATE TABLE IF NOT EXISTS guild_members (
  guild_id  uuid,
  user_id   uuid,
  joined_at timestamp,
  PRIMARY KEY ((guild_id), user_id)
);

CREATE TABLE IF NOT EXISTS user_guilds (
  user_id   uuid,
  guild_id  uuid,
  joined_at timestamp,
  PRIMARY KEY ((user_id), guild_id)
);

-- ---------- channels ----------
CREATE TABLE IF NOT EXISTS channels (
  guild_id   uuid,
  channel_id uuid,
  name       text,
  topic      text,
  type       text,        -- 'text' | 'voice' (null treated as 'text')
  created_at timestamp,
  PRIMARY KEY ((guild_id), channel_id)
);

-- Lookup by channel id alone (message endpoints resolve channel -> guild for authz).
CREATE TABLE IF NOT EXISTS channels_by_id (
  channel_id uuid PRIMARY KEY,
  guild_id   uuid,
  name       text,
  topic      text,
  type       text,
  created_at timestamp
);

-- ---------- messages ----------
-- Discord-style time bucketing: bucket = floor(unix_millis / 864000000)  (10 days).
-- Bounded partitions; newest-first reads walk buckets downward.
CREATE TABLE IF NOT EXISTS messages (
  channel_id  uuid,
  bucket      int,
  message_id  timeuuid,
  author_id   uuid,
  content     text,
  attachments list<frozen<attachment>>,
  edited_at   timestamp,
  PRIMARY KEY ((channel_id, bucket), message_id)
) WITH CLUSTERING ORDER BY (message_id DESC);

-- ---------- reactions ----------
-- One partition per message; read all of a message's reactions in a single scan.
CREATE TABLE IF NOT EXISTS reactions (
  channel_id uuid,
  message_id timeuuid,
  emoji      text,
  user_id    uuid,
  created_at timestamp,
  PRIMARY KEY ((channel_id, message_id), emoji, user_id)
);

-- ---------- invites ----------
CREATE TABLE IF NOT EXISTS invites (
  code       text PRIMARY KEY,
  guild_id   uuid,
  inviter_id uuid,
  created_at timestamp
);
