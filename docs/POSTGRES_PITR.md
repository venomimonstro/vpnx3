# PostgreSQL PITR / encrypted WAL archive

Суточный `pg_dump` ограничивает RPO. Для более серьёзного восстановления VPNX3 поддерживает continuous WAL archive.

## Архитектура

PostgreSQL → local encrypted WAL spool → независимая off-site репликация.

`archive_command` не обращается к сети. Он только:

1. получает завершённый WAL segment;
2. шифрует его отдельным `age` recipient;
3. атомарно сохраняет `<wal>.age`;
4. создаёт SHA-256 sidecar;
5. возвращает success PostgreSQL только после локальной фиксации ciphertext.

Пример PostgreSQL:

```conf
wal_level = replica
archive_mode = on
archive_command = 'env VPNX3_WAL_ARCHIVE_DIR=/srv/vpnx3-wal VPNX3_WAL_AGE_RECIPIENT=age1... /usr/local/lib/vpnx3/postgres-archive-wal.sh %p %f'
archive_timeout = 300
```

## Restore command

После загрузки нужных encrypted WAL files в локальный DR-каталог:

```conf
restore_command = 'env VPNX3_WAL_RESTORE_DIR=/srv/vpnx3-wal-restore VPNX3_WAL_AGE_IDENTITY=/offline/wal.agekey /usr/local/lib/vpnx3/postgres-restore-wal.sh %f %p'
```

Recovery target time/LSN задаётся штатными параметрами PostgreSQL.

WAL encryption identity хранится в DR secret escrow; offline trust root остаётся отдельно.
