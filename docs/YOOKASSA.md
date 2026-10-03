# YooKassa adapter

Первый конкретный payment provider adapter — ЮKassa.

## Создание платежа

Клиент отправляет подписанный device-запрос в `POST /api/v1/client/payments` с `plan_id`. Сервер:

1. проверяет identity устройства и request sequence;
2. загружает продаваемую версию тарифа из своей БД;
3. формирует amount самостоятельно — клиент не задаёт цену;
4. создаёт платеж ЮKassa с уникальным Idempotence-Key;
5. записывает `user_id` и `plan_id` в metadata;
6. сохраняет pending payment через общий Billing Core;
7. возвращает только payment_id и confirmation_url.

## Проверка webhook

Webhook не считается доказательством оплаты сам по себе.

Адаптер получает ID объекта из notification, затем делает серверный GET платежа через ЮKassa API с Basic Auth и сверяет:

- актуальный status;
- payment ID;
- amount/currency;
- metadata `user_id` и `plan_id`.

Только после этого создаётся NormalizedEvent и передаётся в идемпотентный Billing Core.

Идентификатор webhook-события формируется как `event + payment_id`, поэтому повторная доставка одного status event не продлевает подписку повторно.

## Переменные

```
VPNX3_YOOKASSA_SHOP_ID=
VPNX3_YOOKASSA_SECRET_KEY=
VPNX3_YOOKASSA_RETURN_URL=https://...
```

Если они не заданы, интеграция ЮKassa отключена, а основной VPNX3 продолжает запускаться.
