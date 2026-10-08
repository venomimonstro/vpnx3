# Внешний журнал безопасности

Control Plane может отправлять копии новых audit-событий во внешний HTTPS/WORM/SIEM endpoint.

## Переменные

- `VPNX3_SECURITY_EXPORT_URL=https://security.example/events`
- `VPNX3_SECURITY_EXPORT_SIGNING_KEY=<32-byte base64url Ed25519 seed>`

Ключ должен отличаться от Configuration/Access/Release signing keys.

Получатель получает JSON envelope:

- `key_id`;
- `payload` — exact JSON event в base64url;
- `signature` — Ed25519 подпись exact payload.

Payload включает `audit_id`, actor/action/resource, UTC timestamp и `prev_hash/entry_hash` основной audit chain.

## Локальная проверка

Получить публичный ключ можно из того же seed штатной утилитой/владельцем ключа. Для локального smoke-test:

```bash
export VPNX3_SECURITY_EXPORT_PUBLIC_KEY="..."
export VPNX3_RECEIVER_ADDR="127.0.0.1:9099"
export VPNX3_RECEIVER_OUTPUT="/tmp/vpnx3-security-events.ndjson"
go run ./cmd/security-receiver
```

Для Control Plane test URL должен быть HTTPS, поэтому локальный receiver обычно ставится за локальным тестовым TLS reverse proxy.

Receiver специально отказывается слушать внешний интерфейс: это тестовая утилита, не production SIEM.

## Readiness

- не настроен exporter → warning;
- pending до 15 минут → ok;
- pending старше 15 минут или >=8 попыток → warning;
- pending старше 2 часов → failed.

Сбой внешнего получателя не блокирует VPN/API: события остаются в outbox и повторяются с capped exponential backoff.
