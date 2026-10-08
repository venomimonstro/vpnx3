# Внешний аудит VPNX3

Control Plane может отправлять каждую новую запись `audit_log` во внешний WORM/SIEM/receiver.

## Формат

HTTP POST JSON содержит:

- `audit_id`;
- `prev_hash`;
- `entry_hash`;
- actor/action/resource/result;
- request id;
- время.

Не экспортируются `before_state`, `after_state` и source IP.

Заголовки:

- `X-VPNX3-Audit-ID`;
- `X-VPNX3-Timestamp`;
- `X-VPNX3-Signature: sha256=<hex>`.

HMAC считается как:

`HMAC-SHA256(secret, timestamp + "\n" + exact_body_bytes)`.

Receiver обязан:

1. проверять HTTPS на своей стороне;
2. проверять HMAC до разбора/сохранения;
3. ограничивать допустимый возраст timestamp;
4. дедуплицировать по `audit_id`;
5. хранить exact body или минимум audit_id/prev_hash/entry_hash;
6. проверять непрерывность цепочки между последовательными событиями;
7. отвечать 2xx только после устойчивой записи события.

## Retry

Control Plane не блокирует основной запрос. Доставка идёт через outbox.

После 10 ошибок событие становится dead-letter. Owner/security-admin может вручную вернуть до 500 записей в очередь через административный интерфейс/API. Requeue сам попадает в audit log.

Локальная проверка webhook:

```bash
cat event.json | python3 scripts/verify-security-export.py \
  --secret "$VPNX3_SECURITY_EXPORT_SECRET" \
  --timestamp "$TIMESTAMP" \
  --signature "$SIGNATURE"
```
