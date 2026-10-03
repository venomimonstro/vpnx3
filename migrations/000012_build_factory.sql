BEGIN;

ALTER TYPE node_role ADD VALUE IF NOT EXISTS 'build_worker';

ALTER TABLE nodes
  ADD COLUMN IF NOT EXISTS build_sequence BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS releases (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  version TEXT NOT NULL UNIQUE,
  source_commit TEXT NOT NULL CHECK (source_commit ~ '^[0-9a-f]{40}$'),
  status TEXT NOT NULL DEFAULT 'building'
    CHECK (status IN ('draft','building','ready','failed','published','withdrawn')),
  notes TEXT NOT NULL DEFAULT '',
  created_by UUID REFERENCES admin_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS build_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  release_id UUID NOT NULL REFERENCES releases(id) ON DELETE CASCADE,
  target TEXT NOT NULL CHECK (target IN ('android_apk','android_aab','chrome_zip','firefox_zip','ios_ipa')),
  status TEXT NOT NULL DEFAULT 'queued'
    CHECK (status IN ('queued','running','succeeded','failed','cancelled')),
  build_worker_id UUID REFERENCES nodes(id),
  attempt INTEGER NOT NULL DEFAULT 0,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  error_summary TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(release_id,target)
);

CREATE INDEX IF NOT EXISTS build_jobs_queue_idx
  ON build_jobs(status,created_at)
  WHERE status='queued';

CREATE TABLE IF NOT EXISTS release_artifacts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  release_id UUID NOT NULL REFERENCES releases(id) ON DELETE CASCADE,
  build_job_id UUID NOT NULL UNIQUE REFERENCES build_jobs(id) ON DELETE CASCADE,
  target TEXT NOT NULL,
  file_name TEXT NOT NULL,
  storage_key TEXT NOT NULL UNIQUE,
  sha256 CHAR(64) NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  signature_info JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;
