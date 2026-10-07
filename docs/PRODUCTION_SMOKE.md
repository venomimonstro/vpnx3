# Production smoke suite

После каждого production deploy запускается ручной smoke-test. CI/GitHub Actions для проекта не требуются.

## Запуск

```bash
export VPNX3_SMOKE_CONTROL_URL="https://CONTROL-ENDPOINT"
export VPNX3_SMOKE_ADMIN_EMAIL="owner@example.com"
export VPNX3_SMOKE_ADMIN_PASSWORD='...'

bash scripts/production-smoke.sh
```

Для строгой приёмки, где warning также блокирует релиз:

```bash
VPNX3_SMOKE_FAIL_ON_WARNING=true bash scripts/production-smoke.sh
```

## Что проверяется

- liveness Control Plane;
- readiness PostgreSQL + artifact storage;
- публичная metadata API;
- Config Signing Key;
- наличие опубликованного signed Configuration Manifest;
- публичные тарифы;
- реальная admin authentication;
- защищённый admin API;
- launch readiness:
  - worker pool;
  - browser ingress;
  - независимые probes;
  - synthetic WireGuard data plane;
  - Build Factory;
  - release signing;
  - платежный адаптер;
  - artifact storage;
- публичный Release Signing Key;
- logout/revocation admin session.

Скрипт не печатает пароль администратора и bearer token.

## Уровни результата

`FAIL` — текущая конфигурация не должна считаться готовой к коммерческому запуску.

`WARN` — базовый VPN может работать, но конкретный продуктовый контур отсутствует или ещё не подтверждён. Например, нет browser ingress или опубликованного release channel.

`OK` — проверка прошла.

## Что этот smoke-test не заменяет

Он не заменяет физические проверки:

- Android APK/AAB на реальном устройстве;
- Chrome/Firefox extension;
- внешний WireGuard traffic;
- YooKassa sandbox;
- failover между реальными провайдерами/сетями;
- нагрузочные тесты.
