BEGIN;

CREATE TABLE IF NOT EXISTS client_registration_rate (
  source_hash BYTEA NOT NULL,
  bucket_started_at TIMESTAMPTZ NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(source_hash,bucket_started_at)
);

CREATE INDEX IF NOT EXISTS client_registration_rate_updated_idx
  ON client_registration_rate(updated_at);

COMMIT;
