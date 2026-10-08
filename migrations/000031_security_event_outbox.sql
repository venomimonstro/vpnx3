BEGIN;

CREATE TABLE IF NOT EXISTS security_event_outbox (
  audit_id BIGINT PRIMARY KEY REFERENCES audit_log(id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','delivered')),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  last_error TEXT,
  delivered_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS security_event_outbox_pending_idx
  ON security_event_outbox(next_attempt_at,audit_id)
  WHERE status='pending';

CREATE OR REPLACE FUNCTION enqueue_security_audit_event()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO security_event_outbox(audit_id)
  VALUES(NEW.id)
  ON CONFLICT(audit_id) DO NOTHING;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS audit_log_security_outbox_after_insert ON audit_log;
CREATE TRIGGER audit_log_security_outbox_after_insert
AFTER INSERT ON audit_log
FOR EACH ROW EXECUTE FUNCTION enqueue_security_audit_event();

COMMIT;
