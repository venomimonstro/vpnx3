BEGIN;

-- Repair for repositories/deployments affected by the historical duplicate 000019
-- migration number. This migration is intentionally idempotent and guarantees
-- the end state regardless of which old 000019 file was recorded as applied.

CREATE TABLE IF NOT EXISTS client_registration_rate (
  source_hash BYTEA NOT NULL,
  bucket_started_at TIMESTAMPTZ NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(source_hash,bucket_started_at)
);

CREATE INDEX IF NOT EXISTS client_registration_rate_updated_idx
  ON client_registration_rate(updated_at);

CREATE TABLE IF NOT EXISTS device_pairing_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_by_device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  code_hash BYTEA NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS device_pairing_codes_active_idx
  ON device_pairing_codes(expires_at)
  WHERE used_at IS NULL;

DROP TABLE IF EXISTS device_link_codes;

COMMIT;
