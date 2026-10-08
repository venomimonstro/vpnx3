BEGIN;

CREATE TABLE IF NOT EXISTS release_policies (
  target TEXT PRIMARY KEY,
  minimum_supported_version TEXT NOT NULL DEFAULT '',
  recommended_version TEXT NOT NULL DEFAULT '',
  rollout_percent INTEGER NOT NULL DEFAULT 100 CHECK (rollout_percent BETWEEN 0 AND 100),
  blocked_versions TEXT[] NOT NULL DEFAULT '{}',
  message TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_by UUID REFERENCES admin_users(id)
);

CREATE TABLE IF NOT EXISTS release_policy_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  target TEXT NOT NULL,
  minimum_supported_version TEXT NOT NULL,
  recommended_version TEXT NOT NULL,
  rollout_percent INTEGER NOT NULL,
  blocked_versions TEXT[] NOT NULL,
  message TEXT NOT NULL,
  admin_user_id UUID REFERENCES admin_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS release_policy_events_target_created_idx
  ON release_policy_events(target,created_at DESC);

COMMIT;
