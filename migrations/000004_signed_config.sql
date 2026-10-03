BEGIN;

INSERT INTO permissions(code,name) VALUES
  ('config.read','Просмотр конфигурации'),
  ('config.manage','Публикация конфигурации')
ON CONFLICT(code) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.code='owner' AND p.code IN ('config.read','config.manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r JOIN permissions p ON p.code IN ('config.read','config.manage')
WHERE r.code='infrastructure'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r JOIN permissions p ON p.code IN ('config.read')
WHERE r.code IN ('security_admin','analyst','read_only')
ON CONFLICT DO NOTHING;

CREATE SEQUENCE IF NOT EXISTS config_manifest_version_seq START 1;

CREATE TABLE IF NOT EXISTS config_manifests (
  version BIGINT PRIMARY KEY,
  payload JSONB NOT NULL,
  signature BYTEA NOT NULL,
  key_id TEXT NOT NULL,
  created_by UUID REFERENCES admin_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS config_manifests_created_idx ON config_manifests(created_at DESC);

COMMIT;
