BEGIN;

ALTER TABLE nodes
  ADD COLUMN IF NOT EXISTS identity_public_key BYTEA UNIQUE,
  ADD COLUMN IF NOT EXISTS identity_algorithm TEXT,
  ADD COLUMN IF NOT EXISTS heartbeat_sequence BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS enrolled_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS nodes_identity_algorithm_idx ON nodes(identity_algorithm);

COMMIT;
