# Off-site backup replication

Локальный backup не считается достаточной защитой от потери control-host, диска или аккаунта провайдера.

`vpnx3-backup-replicator` берёт уже зашифрованный `.tar.age`, проверяет локальный SHA-256, помещает его в отдельный S3-compatible bucket и затем скачивает объект обратно для повторной проверки SHA-256.

## Инварианты

- в off-site bucket попадает только ciphertext `.tar.age`;
- ключ `age` для расшифровки туда не передаётся;
- S3 credentials отдельны от основного artifact storage;
- имя объекта содержит timestamp backup и не перезаписывается;
- если объект с тем же именем уже существует и hash отличается — операция завершается ошибкой;
- success-marker обновляется только после обратной проверки remote object.

Рекомендуется использовать другой аккаунт/провайдера и включить provider-side Object Lock/versioning, если он доступен.

## Переменные

```bash
VPNX3_BACKUP_SOURCE_DIR=/srv/vpnx3-backups
VPNX3_BACKUP_OFFSITE_STATUS_FILE=/var/lib/vpnx3/backup/offsite-last-success
VPNX3_BACKUP_S3_ENDPOINT=https://s3.example
VPNX3_BACKUP_S3_REGION=eu-1
VPNX3_BACKUP_S3_BUCKET=vpnx3-dr
VPNX3_BACKUP_S3_ACCESS_KEY=...
VPNX3_BACKUP_S3_SECRET_KEY=...
VPNX3_BACKUP_S3_PREFIX=control-backups
```

Минимум одна off-site копия должна находиться вне account/provider основного Control Plane. Recovery identity `age` хранится отдельно от production и backup storage.
