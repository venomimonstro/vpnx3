BEGIN;

CREATE TABLE IF NOT EXISTS client_lease_rate (
  device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('access','proxy')),
  bucket_started_at TIMESTAMPTZ NOT NULL,
  attempts INTEGER NOT NULL CHECK (attempts >= 0),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(device_id,kind,bucket_started_at)
);

CREATE INDEX IF NOT EXISTS client_lease_rate_updated_idx
  ON client_lease_rate(updated_at);

COMMIT;
