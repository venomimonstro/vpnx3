BEGIN;

ALTER TABLE client_telemetry_daily
  ADD COLUMN IF NOT EXISTS worker_node_key TEXT NOT NULL DEFAULT '';

UPDATE client_telemetry_daily
SET worker_node_key=COALESCE(worker_node_id::text,'')
WHERE worker_node_key='';

ALTER TABLE client_telemetry_daily
  DROP CONSTRAINT IF EXISTS client_telemetry_daily_pkey;

ALTER TABLE client_telemetry_daily
  ALTER COLUMN worker_node_id DROP NOT NULL;

ALTER TABLE client_telemetry_daily
  ADD PRIMARY KEY(
    day,platform,client_version,event_type,config_version,worker_node_key,network_type
  );

COMMIT;
