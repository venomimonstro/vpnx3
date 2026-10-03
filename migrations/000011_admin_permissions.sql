BEGIN;

INSERT INTO permissions (code,name) VALUES
  ('config.manage','Управление подписанной конфигурацией'),
  ('audit.read','Просмотр журнала аудита')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.code='owner'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id
FROM roles r JOIN permissions p ON p.code IN ('config.manage')
WHERE r.code='infrastructure'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id
FROM roles r JOIN permissions p ON p.code IN ('audit.read')
WHERE r.code IN ('security_admin','analyst','read_only')
ON CONFLICT DO NOTHING;

COMMIT;
