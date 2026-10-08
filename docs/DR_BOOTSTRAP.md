# DR bootstrap

`scripts/dr-bootstrap.sh` собирает отдельные операции восстановления в один fail-closed сценарий.

Он выполняет:

1. проверку encrypted backup;
2. restore только в явно подтверждённую изолированную БД;
3. применение актуальных migrations без зависимости от runtime signing secrets;
4. проверку `verify_audit_chain()`;
5. проверку критических таблиц последних спринтов;
6. вывод контрольных бизнес-счётчиков.

Скрипт **не запускает публичный Control Plane** и не меняет DNS/endpoints.

## Запуск

```bash
export VPNX3_BACKUP_FILE=/recovery/vpnx3-....tar.age
export VPNX3_BACKUP_AGE_IDENTITY=/offline/data-backup.agekey
export VPNX3_RESTORE_DATABASE_URL='postgres://.../vpnx3_dr?sslmode=require'
export VPNX3_DR_CONFIRM=DR_ISOLATED_ENVIRONMENT
export VPNX3_MIGRATE_BINARY=/usr/local/bin/vpnx3-migrate
export VPNX3_MIGRATIONS_DIR=/opt/vpnx3/migrations

bash scripts/dr-bootstrap.sh
```

После PASS необходимо отдельно:

- получить DR secret escrow;
- с offline Root of Trust выпустить/подтвердить operational signing keys;
- установить актуальный signed trust bundle;
- поднять Control Plane только в private management network;
- пройти `/api/v1/admin/readiness`;
- проверить config/revocation/release mirrors;
- только после этого публиковать новые ingress/client endpoints.
