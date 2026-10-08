BEGIN;

ALTER TABLE security_event_outbox
  DROP CONSTRAINT IF EXISTS security_event_outbox_status_check;

ALTER TABLE security_event_outbox
  ADD CONSTRAINT security_event_outbox_status_check
  CHECK (status IN ('pending','delivered','dead'));

CREATE INDEX IF NOT EXISTS security_event_outbox_dead_idx
  ON security_event_outbox(audit_id)
  WHERE status='dead';

COMMIT;
