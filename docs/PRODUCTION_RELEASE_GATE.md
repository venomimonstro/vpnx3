# Production Release Gate

В `VPNX3_ENV=production` публикация релиза сервером блокируется, если нарушен любой критический инвариант:

- свежий signed Configuration Manifest отсутствует или старше 2 часов;
- нет routable VPN worker;
- нет свежего independent probe;
- нет успешного synthetic WireGuard data-plane observation;
- audit hash chain повреждена;
- Release Signing Key отсутствует;
- YooKassa не настроена;
- backup marker отсутствует или старше 36 часов;
- artifact storage недоступен;
- build job релиза не завершён успешно;
- browser package публикуется без active ingress.

Ошибки auto-renew за 24 часа сейчас являются предупреждением, потому что единичный отказ карты пользователя не означает системную недоступность billing.

В development/staging gate показывает те же сигналы, но не блокирует publish. Это позволяет локально собирать и тестировать релизы без production-инфраструктуры.
