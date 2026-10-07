# Failover drill

`scripts/failover-drill.sh` проверяет реальный Control Plane routing lifecycle без CI.

Сценарий:

1. проверяет окружение;
2. авторизуется администратором;
3. убеждается, что выбранная worker/ingress нода active;
4. требует минимум две маршрутизируемые ноды соответствующей роли;
5. фиксирует текущую версию подписанного Configuration Manifest;
6. переводит выбранную ноду в `draining`;
7. ждёт новую версию manifest и проверяет, что нода исчезла из клиентского пула;
8. возвращает ноду в `active`;
9. ждёт ещё одну версию manifest и проверяет возврат ноды;
10. повторно запускает launch readiness.

Если скрипт аварийно завершается после drain, trap делает best-effort возврат ноды в active.

## Безопасность

По умолчанию production блокируется.

Для staging:

```bash
export VPNX3_DRILL_CONTROL_URL="https://staging-control.example"
export VPNX3_DRILL_ADMIN_EMAIL="owner@example.com"
export VPNX3_DRILL_ADMIN_PASSWORD='...'
export VPNX3_DRILL_NODE_ID="UUID"

bash scripts/failover-drill.sh
```

Для production необходимо явно указать:

```bash
export VPNX3_DRILL_ALLOW_PRODUCTION=YES
```

Перед production drill необходимо убедиться, что второй worker/ingress физически находится на независимой инфраструктуре и выдержит текущую нагрузку.
