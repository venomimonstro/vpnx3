# DR secret escrow

Обычный backup содержит данные, но намеренно не содержит operational secrets.

Для восстановления после полной потери Control Plane используется отдельный зашифрованный secret escrow.

## Что допустимо помещать

Только явно выбранные recovery-материалы, например:

- recovery identity для расшифровки data backup;
- recovery credentials платёжного провайдера;
- credentials независимого DR/S3 аккаунта;
- provider recovery codes;
- документ/файл с bootstrap-параметрами нового Control Plane.

## Что запрещено помещать вместе

**Offline Root of Trust private seed не должен находиться в этом escrow.**

Root seed хранится отдельно/offline. При полном DR через него выпускается новый root-signed trust bundle для новых operational config/access/release keys.

## Создание

```bash
bash scripts/create-dr-secret-escrow.sh \
  --recipient age1DIFFERENTRECIPIENT... \
  --out /offline/vpnx3-dr-secrets-20261008.tar.age \
  --file backup_age_identity=/offline/data-backup.agekey \
  --file provider_recovery=/offline/provider-recovery.txt
```

Каждый source-файл должен быть обычным файлом с mode 0400/0600. Symlink запрещён.

## Проверка

```bash
export VPNX3_DR_ESCROW_FILE=/offline/vpnx3-dr-secrets-20261008.tar.age
export VPNX3_DR_ESCROW_AGE_IDENTITY=/offline/dr-escrow.agekey
bash scripts/verify-dr-secret-escrow.sh
```

Проверяется внешний SHA-256 ciphertext и manifest/checksum каждого файла после расшифровки.

## DR-последовательность

1. получить encrypted data backup из независимого off-site storage;
2. независимо получить DR secret escrow и его age identity;
3. восстановить PostgreSQL в новую инфраструктуру;
4. с offline Root of Trust создать новые operational keys и новый trust bundle;
5. восстановить/заменить credentials внешних провайдеров;
6. поднять Control Plane в изолированном режиме;
7. проверить migrations/readiness/audit chain;
8. только затем публиковать новые client endpoints.
