BEGIN;
CREATE TABLE IF NOT EXISTS node_endpoints (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('session_api','wireguard','ingress')),
  transport TEXT NOT NULL,
  scheme TEXT NOT NULL CHECK (scheme IN ('https','udp')),
  host TEXT NOT NULL CHECK (length(host) BETWEEN 1 AND 255),
  port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
  path TEXT NOT NULL DEFAULT '',
  priority INTEGER NOT NULL DEFAULT 100 CHECK (priority BETWEEN 0 AND 10000),
  enabled BOOLEAN NOT NULL DEFAULT true,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(node_id,kind,transport,host,port,path)
);
CREATE INDEX IF NOT EXISTS node_endpoints_node_idx ON node_endpoints(node_id,enabled,priority);
COMMIT;
