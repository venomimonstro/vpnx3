BEGIN;

CREATE TABLE IF NOT EXISTS client_telemetry_daily (
  day DATE NOT NULL,
  platform TEXT NOT NULL,
  client_version TEXT NOT NULL,
  event_type TEXT NOT NULL,
  config_version BIGINT NOT NULL DEFAULT 0,
  worker_node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
  network_type TEXT NOT NULL DEFAULT 'unknown',
  event_count BIGINT NOT NULL DEFAULT 0 CHECK (event_count >= 0),
  duration_ms_sum BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms_sum >= 0),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(day,platform,client_version,event_type,config_version,worker_node_id,network_type)
);

CREATE INDEX IF NOT EXISTS client_telemetry_daily_day_idx
  ON client_telemetry_daily(day DESC);

COMMIT;
