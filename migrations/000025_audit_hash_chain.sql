BEGIN;

ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS prev_hash BYTEA;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS entry_hash BYTEA;

DROP TRIGGER IF EXISTS audit_log_no_update ON audit_log;

CREATE OR REPLACE FUNCTION audit_log_canonical(
  p_prev BYTEA,
  p_actor_type TEXT,
  p_actor_id TEXT,
  p_action TEXT,
  p_resource_type TEXT,
  p_resource_id TEXT,
  p_request_id TEXT,
  p_source_ip INET,
  p_before JSONB,
  p_after JSONB,
  p_result TEXT,
  p_created TIMESTAMPTZ
) RETURNS BYTEA
LANGUAGE sql IMMUTABLE AS $$
  SELECT digest(
    COALESCE(p_prev,''::bytea) ||
    convert_to(
      concat_ws(E'\x1f',
        COALESCE(p_actor_type,''),
        COALESCE(p_actor_id,''),
        COALESCE(p_action,''),
        COALESCE(p_resource_type,''),
        COALESCE(p_resource_id,''),
        COALESCE(p_request_id,''),
        COALESCE(p_source_ip::text,''),
        COALESCE(p_before,'null'::jsonb)::text,
        COALESCE(p_after,'null'::jsonb)::text,
        COALESCE(p_result,''),
        to_char(p_created AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
      ),
      'UTF8'
    ),
    'sha256'
  )
$$;

DO $$
DECLARE
  rec RECORD;
  previous BYTEA := ''::bytea;
  calculated BYTEA;
BEGIN
  FOR rec IN SELECT * FROM audit_log ORDER BY id LOOP
    calculated := audit_log_canonical(
      previous,rec.actor_type,rec.actor_id,rec.action,rec.resource_type,
      rec.resource_id,rec.request_id,rec.source_ip,rec.before_state,
      rec.after_state,rec.result,rec.created_at
    );
    UPDATE audit_log SET prev_hash=previous,entry_hash=calculated WHERE id=rec.id;
    previous := calculated;
  END LOOP;
END;
$$;

ALTER TABLE audit_log ALTER COLUMN prev_hash SET DEFAULT ''::bytea;
UPDATE audit_log SET prev_hash=''::bytea WHERE prev_hash IS NULL;
ALTER TABLE audit_log ALTER COLUMN prev_hash SET NOT NULL;
ALTER TABLE audit_log ALTER COLUMN entry_hash SET NOT NULL;

CREATE OR REPLACE FUNCTION audit_log_chain_insert()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  previous BYTEA;
BEGIN
  PERFORM pg_advisory_xact_lock(915731042);
  SELECT entry_hash INTO previous FROM audit_log ORDER BY id DESC LIMIT 1;
  NEW.prev_hash := COALESCE(previous,''::bytea);
  NEW.entry_hash := audit_log_canonical(
    NEW.prev_hash,NEW.actor_type,NEW.actor_id,NEW.action,NEW.resource_type,
    NEW.resource_id,NEW.request_id,NEW.source_ip,NEW.before_state,
    NEW.after_state,NEW.result,NEW.created_at
  );
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS audit_log_chain_before_insert ON audit_log;
CREATE TRIGGER audit_log_chain_before_insert
BEFORE INSERT ON audit_log
FOR EACH ROW EXECUTE FUNCTION audit_log_chain_insert();

CREATE OR REPLACE FUNCTION verify_audit_chain()
RETURNS BOOLEAN LANGUAGE plpgsql STABLE AS $$
DECLARE
  rec RECORD;
  previous BYTEA := ''::bytea;
  calculated BYTEA;
BEGIN
  FOR rec IN SELECT * FROM audit_log ORDER BY id LOOP
    IF rec.prev_hash IS DISTINCT FROM previous THEN
      RETURN false;
    END IF;
    calculated := audit_log_canonical(
      previous,rec.actor_type,rec.actor_id,rec.action,rec.resource_type,
      rec.resource_id,rec.request_id,rec.source_ip,rec.before_state,
      rec.after_state,rec.result,rec.created_at
    );
    IF rec.entry_hash IS DISTINCT FROM calculated THEN
      RETURN false;
    END IF;
    previous := rec.entry_hash;
  END LOOP;
  RETURN true;
END;
$$;

CREATE OR REPLACE FUNCTION deny_audit_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'audit_log is append-only';
END;
$$;

CREATE TRIGGER audit_log_no_update
BEFORE UPDATE OR DELETE ON audit_log
FOR EACH ROW EXECUTE FUNCTION deny_audit_mutation();

COMMIT;
