BEGIN;

ALTER TABLE devices
  ADD COLUMN IF NOT EXISTS identity_public_key BYTEA UNIQUE,
  ADD COLUMN IF NOT EXISTS identity_algorithm TEXT,
  ADD COLUMN IF NOT EXISTS request_sequence BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS trial_started_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS trial_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS devices_trial_expires_idx ON devices(trial_expires_at);

COMMIT;
