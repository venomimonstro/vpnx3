BEGIN;

ALTER TABLE incident_notification_outbox
  DROP CONSTRAINT IF EXISTS incident_notification_outbox_status_check;

ALTER TABLE incident_notification_outbox
  ADD CONSTRAINT incident_notification_outbox_status_check
  CHECK (status IN ('pending','delivered','dead'));

CREATE INDEX IF NOT EXISTS incident_notification_outbox_dead_idx
  ON incident_notification_outbox(id)
  WHERE status='dead';

COMMIT;
