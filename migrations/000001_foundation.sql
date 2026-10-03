BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE node_role AS ENUM ('ingress', 'worker', 'probe', 'config_mirror');
CREATE TYPE node_status AS ENUM (
  'new',
  'enrolling',
  'provisioning',
  'testing',
  'draft',
  'active',
  'degraded',
  'draining',
  'maintenance',
  'quarantined',
  'retired',
  'destroyed'
);

CREATE TABLE admin_users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE admin_user_roles (
  admin_user_id UUID NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
  role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  PRIMARY KEY (admin_user_id, role_id)
);

INSERT INTO roles (code, name) VALUES
  ('owner', 'Владелец'),
  ('security_admin', 'Специалист безопасности'),
  ('infrastructure', 'Руководитель инфраструктуры'),
  ('finance', 'Финансы'),
  ('support', 'Поддержка'),
  ('analyst', 'Аналитик'),
  ('read_only', 'Наблюдатель')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  status TEXT NOT NULL DEFAULT 'active',
  email TEXT UNIQUE,
  phone TEXT UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ
);

CREATE TABLE devices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  platform TEXT NOT NULL,
  display_name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  client_version TEXT,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ
);

CREATE INDEX devices_user_id_idx ON devices(user_id);

CREATE TABLE plans (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code TEXT NOT NULL,
  version INTEGER NOT NULL,
  name TEXT NOT NULL,
  price_minor BIGINT NOT NULL CHECK (price_minor >= 0),
  currency CHAR(3) NOT NULL DEFAULT 'RUB',
  billing_period_days INTEGER NOT NULL CHECK (billing_period_days > 0),
  device_limit INTEGER NOT NULL CHECK (device_limit > 0),
  traffic_limit_bytes BIGINT,
  trial_days INTEGER NOT NULL DEFAULT 0 CHECK (trial_days >= 0),
  grace_days INTEGER NOT NULL DEFAULT 0 CHECK (grace_days >= 0),
  sale_enabled BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(code, version)
);

CREATE TABLE subscriptions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plan_id UUID NOT NULL REFERENCES plans(id),
  status TEXT NOT NULL,
  starts_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  grace_until TIMESTAMPTZ,
  auto_renew BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX subscriptions_user_id_idx ON subscriptions(user_id);
CREATE INDEX subscriptions_status_expires_idx ON subscriptions(status, expires_at);

CREATE TABLE nodes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL UNIQUE,
  role node_role NOT NULL,
  status node_status NOT NULL DEFAULT 'new',
  country_code CHAR(2),
  provider TEXT,
  provider_instance_id TEXT,
  public_ip INET,
  private_ip INET,
  agent_version TEXT,
  capacity_sessions INTEGER CHECK (capacity_sessions IS NULL OR capacity_sessions > 0),
  current_sessions INTEGER NOT NULL DEFAULT 0 CHECK (current_sessions >= 0),
  monthly_cost_minor BIGINT CHECK (monthly_cost_minor IS NULL OR monthly_cost_minor >= 0),
  currency CHAR(3) NOT NULL DEFAULT 'EUR',
  health_score NUMERIC(5,2) CHECK (health_score IS NULL OR (health_score >= 0 AND health_score <= 100)),
  last_heartbeat_at TIMESTAMPTZ,
  published_at TIMESTAMPTZ,
  quarantined_at TIMESTAMPTZ,
  retired_at TIMESTAMPTZ,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX nodes_status_idx ON nodes(status);
CREATE INDEX nodes_role_status_idx ON nodes(role, status);
CREATE INDEX nodes_provider_idx ON nodes(provider);

CREATE TABLE node_state_events (
  id BIGSERIAL PRIMARY KEY,
  node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  previous_status node_status,
  next_status node_status NOT NULL,
  reason TEXT,
  actor_type TEXT NOT NULL,
  actor_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX node_state_events_node_created_idx ON node_state_events(node_id, created_at DESC);

CREATE TABLE enrollment_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token_hash BYTEA NOT NULL UNIQUE,
  intended_role node_role NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  created_by UUID REFERENCES admin_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX enrollment_tokens_expires_idx ON enrollment_tokens(expires_at) WHERE used_at IS NULL;

CREATE TABLE incidents (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  severity TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open',
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  scope JSONB NOT NULL DEFAULT '{}'::jsonb,
  detected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ,
  root_cause TEXT
);

CREATE TABLE audit_log (
  id BIGSERIAL PRIMARY KEY,
  actor_type TEXT NOT NULL,
  actor_id TEXT,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT,
  request_id TEXT,
  source_ip INET,
  before_state JSONB,
  after_state JSONB,
  result TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_created_idx ON audit_log(created_at DESC);
CREATE INDEX audit_log_resource_idx ON audit_log(resource_type, resource_id, created_at DESC);

COMMIT;
