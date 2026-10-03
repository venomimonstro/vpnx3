BEGIN;

CREATE TABLE IF NOT EXISTS permissions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS role_permissions (
  role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
  PRIMARY KEY (role_id, permission_id)
);

INSERT INTO permissions (code, name) VALUES
  ('admin.manage', 'Управление администраторами'),
  ('security.manage', 'Управление безопасностью'),
  ('nodes.read', 'Просмотр узлов'),
  ('nodes.manage', 'Управление узлами'),
  ('users.read', 'Просмотр пользователей'),
  ('users.manage', 'Управление пользователями'),
  ('billing.read', 'Просмотр платежей'),
  ('billing.manage', 'Управление платежами'),
  ('analytics.read', 'Просмотр аналитики'),
  ('incidents.read', 'Просмотр инцидентов'),
  ('incidents.manage', 'Управление инцидентами'),
  ('releases.read', 'Просмотр выпусков'),
  ('releases.manage', 'Управление выпусками')
ON CONFLICT (code) DO NOTHING;

-- Владелец получает все разрешения.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.code = 'owner'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.code IN
  ('security.manage','nodes.read','incidents.read','incidents.manage','releases.read')
WHERE r.code = 'security_admin'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.code IN
  ('nodes.read','nodes.manage','analytics.read','incidents.read','incidents.manage')
WHERE r.code = 'infrastructure'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.code IN
  ('billing.read','billing.manage','analytics.read','users.read')
WHERE r.code = 'finance'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.code IN
  ('users.read','users.manage','billing.read','incidents.read')
WHERE r.code = 'support'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.code IN
  ('analytics.read','nodes.read','users.read','billing.read','incidents.read','releases.read')
WHERE r.code = 'analyst'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.code IN
  ('analytics.read','nodes.read','users.read','incidents.read','releases.read')
WHERE r.code = 'read_only'
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS admin_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  admin_user_id UUID NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
  token_hash BYTEA NOT NULL UNIQUE,
  user_agent TEXT,
  source_ip INET,
  expires_at TIMESTAMPTZ NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS admin_sessions_user_idx ON admin_sessions(admin_user_id);
CREATE INDEX IF NOT EXISTS admin_sessions_active_idx ON admin_sessions(expires_at) WHERE revoked_at IS NULL;

CREATE OR REPLACE FUNCTION deny_audit_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'audit_log is append-only';
END;
$$;

DROP TRIGGER IF EXISTS audit_log_no_update ON audit_log;
CREATE TRIGGER audit_log_no_update
BEFORE UPDATE OR DELETE ON audit_log
FOR EACH ROW EXECUTE FUNCTION deny_audit_mutation();

COMMIT;
