BEGIN;

ALTER TABLE nodes
  ADD COLUMN IF NOT EXISTS probe_sequence BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS probe_results (
  id BIGSERIAL PRIMARY KEY,
  probe_node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  target_node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  endpoint_kind TEXT NOT NULL,
  success BOOLEAN NOT NULL,
  latency_ms INTEGER NOT NULL CHECK (latency_ms >= 0),
  observed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS probe_results_target_time_idx
  ON probe_results(target_node_id, observed_at DESC);

CREATE INDEX IF NOT EXISTS probe_results_probe_time_idx
  ON probe_results(probe_node_id, observed_at DESC);

COMMIT;
