BEGIN;

ALTER TABLE config_manifests
  ADD COLUMN IF NOT EXISTS payload_raw BYTEA;

COMMENT ON COLUMN config_manifests.payload_raw IS
  'Exact bytes that were cryptographically signed. Never reconstruct from JSONB.';

COMMIT;
