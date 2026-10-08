BEGIN;

CREATE TABLE IF NOT EXISTS incident_notification_outbox (
  id BIGSERIAL PRIMARY KEY,
  incident_id UUID NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL CHECK (event_type IN ('created','updated','resolved')),
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','delivered')),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  last_error TEXT,
  delivered_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS incident_notification_outbox_pending_idx
  ON incident_notification_outbox(next_attempt_at,id)
  WHERE status='pending';

CREATE OR REPLACE FUNCTION enqueue_incident_notification()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  kind TEXT;
BEGIN
  IF TG_OP='INSERT' THEN
    kind := 'created';
  ELSIF NEW.status='resolved' AND OLD.status IS DISTINCT FROM NEW.status THEN
    kind := 'resolved';
  ELSIF OLD.status IS DISTINCT FROM NEW.status
     OR OLD.severity IS DISTINCT FROM NEW.severity
     OR OLD.title IS DISTINCT FROM NEW.title
     OR OLD.summary IS DISTINCT FROM NEW.summary
     OR OLD.root_cause IS DISTINCT FROM NEW.root_cause THEN
    kind := 'updated';
  ELSE
    RETURN NEW;
  END IF;

  INSERT INTO incident_notification_outbox(incident_id,event_type,payload)
  VALUES(
    NEW.id,
    kind,
    jsonb_build_object(
      'id',NEW.id,
      'severity',NEW.severity,
      'status',NEW.status,
      'title',NEW.title,
      'summary',NEW.summary,
      'detected_at',NEW.detected_at,
      'resolved_at',NEW.resolved_at,
      'root_cause',NEW.root_cause
    )
  );
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS incidents_notification_after_change ON incidents;
CREATE TRIGGER incidents_notification_after_change
AFTER INSERT OR UPDATE ON incidents
FOR EACH ROW EXECUTE FUNCTION enqueue_incident_notification();

COMMIT;
