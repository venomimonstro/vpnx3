BEGIN;

ALTER TABLE nodes
  ADD COLUMN IF NOT EXISTS circuit_breaker_open BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS nodes_circuit_breaker_idx
  ON nodes(circuit_breaker_open)
  WHERE circuit_breaker_open=true;

COMMIT;
