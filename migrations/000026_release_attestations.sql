BEGIN;

CREATE TABLE IF NOT EXISTS release_publication_attestations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  release_id UUID NOT NULL REFERENCES releases(id) ON DELETE CASCADE,
  admin_user_id UUID REFERENCES admin_users(id) ON DELETE SET NULL,
  environment TEXT NOT NULL,
  gate_status TEXT NOT NULL,
  blockers JSONB NOT NULL DEFAULT '[]'::jsonb,
  warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
  signals JSONB NOT NULL DEFAULT '{}'::jsonb,
  checked_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS release_attestations_release_created_idx
  ON release_publication_attestations(release_id,created_at DESC);

CREATE OR REPLACE FUNCTION deny_release_attestation_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'release publication attestations are append-only';
END;
$$;

DROP TRIGGER IF EXISTS release_attestations_no_update ON release_publication_attestations;
CREATE TRIGGER release_attestations_no_update
BEFORE UPDATE OR DELETE ON release_publication_attestations
FOR EACH ROW EXECUTE FUNCTION deny_release_attestation_mutation();

COMMIT;
