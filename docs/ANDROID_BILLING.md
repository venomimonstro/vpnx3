# Android billing flow

Android теперь использует существующий server-side Billing Core.

Путь:

1. `GET /api/v1/plans` — приложение получает только продаваемые версии тарифов.
2. Пользователь выбирает тариф.
3. Android резервирует следующий device request sequence.
4. `POST /api/v1/client/payments` отправляет подписанный device-запрос с **только plan_id**.
5. Control Plane самостоятельно загружает plan price/currency из БД и создаёт платёж ЮKassa.
6. Android получает `confirmation_url` и открывает системный браузер.

Приложение не отправляет сумму и не может изменить цену.

После подтверждения webhook сервер создаёт/продлевает subscription. Следующее получение Access Lease уже использует новое entitlement.

На этом этапе возврат из браузера не считается доказательством оплаты — источником истины остаётся verified provider webhook/API.
