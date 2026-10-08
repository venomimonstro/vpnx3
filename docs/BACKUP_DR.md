# Backup / Disaster Recovery

## Цель

Backup считается рабочим только после успешного restore drill.

### Что входит

- PostgreSQL в custom-format `pg_dump`;
- опционально local artifact volume;
- manifest с SHA-256 и размерами;
- весь bundle шифруется `age` до записи постоянного файла.

### Что намеренно НЕ входит

Никогда не архивируются автоматически:

- Configuration Signing Key;
- Access Lease Signing Key;
- Release Signing Key;
- YooKassa secret;
- S3 secret;
- Apple/Android signing credentials.

Они должны храниться в отдельном off-host secret escrow с независимым контролем доступа. Иначе кража backup одновременно раскрывает и данные, и корневые ключи доверия.

## Backup

```bash
export VPNX3_BACKUP_DATABASE_URL='postgres://...'
export VPNX3_BACKUP_DIR='/srv/vpnx3-backups'
export VPNX3_BACKUP_AGE_RECIPIENT='age1...'
export VPNX3_BACKUP_RETENTION_DAYS=35

bash scripts/backup-control-state.sh
```

Для local artifact storage:

```bash
export VPNX3_BACKUP_INCLUDE_ARTIFACTS=yes
export VPNX3_BACKUP_ARTIFACT_DIR=/var/lib/vpnx3/artifacts
```

При S3-compatible artifact storage отдельная копия bucket должна делаться средствами storage/provider; исходники релизов всё равно закреплены exact Git commit SHA.

## Проверка архива

```bash
export VPNX3_BACKUP_FILE=/srv/vpnx3-backups/vpnx3-....tar.age
export VPNX3_BACKUP_AGE_IDENTITY=/secure/backup.agekey
bash scripts/verify-backup.sh
```

## Restore drill

Использовать только отдельную пустую/тестовую БД:

```bash
export VPNX3_RESTORE_DATABASE_URL='postgres://.../vpnx3_restore_drill?sslmode=require'
export VPNX3_RESTORE_CONFIRM=RESTORE_ISOLATED_DATABASE
bash scripts/restore-drill.sh
```

После восстановления необходимо отдельно поднять Control Plane с **копиями тестовых** signing keys или с production key escrow в контролируемом DR-окне, проверить migrations/readiness и затем удалить drill environment.

## Минимальная политика

- backup не реже раза в сутки;
- хранить минимум 7 последних ежедневных копий и несколько недельных;
- минимум одна копия — у другого провайдера/в другом аккаунте;
- restore drill минимум раз в месяц и после изменения схемы backup;
- успешность backup/restore фиксировать в операционном журнале;
- recovery ключ `age` не хранить на том же сервере, где лежат backup bundles.
