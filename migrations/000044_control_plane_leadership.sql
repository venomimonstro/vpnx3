BEGIN;

CREATE TABLE IF NOT EXISTS control_plane_leadership (
  name TEXT PRIMARY KEY,
  holder_id TEXT,
  acquired_at TIMESTAMPTZ,
  heartbeat_at TIMESTAMPTZ,
  transitions BIGINT NOT NULL DEFAULT 0 CHECK (transitions >= 0),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;
