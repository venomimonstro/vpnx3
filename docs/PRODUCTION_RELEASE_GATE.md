# Production Release Gate

В `VPNX3_ENV=production` публикация релиза сервером выполняется fail-closed.

Критические блокеры:

- signed Configuration Manifest отсутствует или старше 2 часов;
- нет routable VPN worker;
- нет свежего independent probe;
- нет успешного synthetic WireGuard data-plane observation;
- audit hash chain повреждена;
- Release Signing Key отсутствует;
- ЮKassa не настроена;
- YooKassa circuit breaker открыт;
- primary PostgreSQL подключён не к writer;
- primary DB pool занят на 95% или больше;
- HTTP admission занят на 95% или больше;
- при strict DB HA standby отсутствует/недоступен/не является standby;
- при strict DB HA WAL lag standby больше 512 МБ;
- локальный backup отсутствует или старше 36 часов;
- off-site backup отсутствует или старше 36 часов;
- WAL off-site replication отсутствует или не подтверждалась больше 30 минут;
- artifact storage недоступен;
- build job релиза не завершён успешно;
- browser package публикуется без active ingress.

Предупреждения, не блокирующие publish сами по себе:

- YooKassa circuit half-open;
- DB pool/admission в диапазоне 80–95%;
- standby WAL lag 64–512 МБ;
- единичные ошибки auto-renew за 24 часа.

В development/staging те же критические условия записываются как warning и не блокируют publish.

При фактическом publish сохраняется append-only release publication attestation с gate status/signals.
