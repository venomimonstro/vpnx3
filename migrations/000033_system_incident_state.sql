BEGIN;

CREATE TABLE IF NOT EXISTS system_incident_state (
  code TEXT PRIMARY KEY,
  incident_id UUID REFERENCES incidents(id) ON DELETE SET NULL,
  active BOOLEAN NOT NULL DEFAULT false,
  last_transition_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;
