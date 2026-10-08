BEGIN;

CREATE TABLE IF NOT EXISTS operational_condition_states (
  code TEXT PRIMARY KEY,
  state TEXT NOT NULL DEFAULT 'unknown'
    CHECK (state IN ('unknown','healthy','unhealthy')),
  consecutive_bad INTEGER NOT NULL DEFAULT 0 CHECK (consecutive_bad >= 0),
  consecutive_good INTEGER NOT NULL DEFAULT 0 CHECK (consecutive_good >= 0),
  active_incident_id UUID REFERENCES incidents(id) ON DELETE SET NULL,
  last_detail TEXT,
  last_checked_at TIMESTAMPTZ,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS operational_condition_incident_idx
  ON operational_condition_states(active_incident_id)
  WHERE active_incident_id IS NOT NULL;

COMMIT;
