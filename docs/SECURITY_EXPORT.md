# Внешний аудит VPNX3

Каждая новая запись `audit_log` автоматически попадает в transactional outbox и независимо доставляется во внешний WORM/SIEM/receiver.

## Модель доверия

Для внешнего аудита используется отдельный Ed25519 signing key:

- приватный seed хранится только на Control Plane;
- receiver получает только публичный ключ;
- ключ обязан отличаться от Configuration, Access и Release signing keys.

Наружу не отправляются `before_state`, `after_state` и source IP. Подписанный payload содержит:

- audit_id;
- prev_hash / entry_hash;
- actor type/id;
- action;
- resource type/id;
- request id;
- result;
- created_at;
- номер попытки доставки.

HTTP body — envelope:

```json
{"key_id":"...","payload":"base64url","signature":"base64url"}
```

Подпись считается Ed25519 по exact decoded `payload` bytes.

Receiver должен:

1. принимать только HTTPS;
2. до сохранения проверять Ed25519 signature и key_id;
3. дедуплицировать по audit_id;
4. проверять непрерывность prev_hash → entry_hash;
5. подтверждать 2xx только после устойчивой записи.

## Retry / dead-letter

Control Plane не блокирует клиентские/admin запросы ожиданием receiver.

После 10 неуспешных доставок запись становится dead-letter. Owner/security-admin может вернуть dead-letter в очередь через админку; requeue сам попадает в audit log.

Локальная проверка envelope:

```bash
cat envelope.json | go run ./cmd/security-export-verify \
  --public-key "$VPNX3_SECURITY_EXPORT_PUBLIC_KEY"
```
