BEGIN;

INSERT INTO permissions(code,name) VALUES
  ('security.export.manage','Управление внешним экспортом аудита')
ON CONFLICT(code) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id
FROM roles r JOIN permissions p ON p.code='security.export.manage'
WHERE r.code IN ('owner','security_admin')
ON CONFLICT DO NOTHING;

COMMIT;
