BEGIN;

CREATE TABLE IF NOT EXISTS device_revocation_events (
  id BIGSERIAL PRIMARY KEY,
  device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL CHECK (event_type IN ('revoked','reactivated')),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS device_revocation_events_device_idx
  ON device_revocation_events(device_id,id DESC);

CREATE OR REPLACE FUNCTION record_device_revocation_transition()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status IS DISTINCT FROM NEW.status THEN
    IF NEW.status='revoked' THEN
      INSERT INTO device_revocation_events(device_id,event_type,occurred_at)
      VALUES(NEW.id,'revoked',COALESCE(NEW.revoked_at,now()));
    ELSIF OLD.status='revoked' AND NEW.status='active' THEN
      INSERT INTO device_revocation_events(device_id,event_type,occurred_at)
      VALUES(NEW.id,'reactivated',now());
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS devices_revocation_after_status_change ON devices;
CREATE TRIGGER devices_revocation_after_status_change
AFTER UPDATE OF status ON devices
FOR EACH ROW EXECUTE FUNCTION record_device_revocation_transition();

INSERT INTO device_revocation_events(device_id,event_type,occurred_at)
SELECT d.id,'revoked',COALESCE(d.revoked_at,now())
FROM devices d
WHERE d.status='revoked'
  AND NOT EXISTS (
    SELECT 1 FROM device_revocation_events e
    WHERE e.device_id=d.id AND e.event_type='revoked'
  );

COMMIT;
