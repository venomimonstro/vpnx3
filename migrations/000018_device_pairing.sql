BEGIN;

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

COMMIT;
