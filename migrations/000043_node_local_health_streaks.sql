BEGIN;

ALTER TABLE nodes
  ADD COLUMN IF NOT EXISTS local_health_bad_streak INTEGER NOT NULL DEFAULT 0 CHECK (local_health_bad_streak >= 0),
  ADD COLUMN IF NOT EXISTS local_health_good_streak INTEGER NOT NULL DEFAULT 0 CHECK (local_health_good_streak >= 0);

COMMIT;
