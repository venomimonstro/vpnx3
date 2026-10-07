# Soak monitor

`scripts/soak-monitor.py` — read-only длительная проверка стабильности после deploy.

Ограничения:

- только HTTPS GET;
- URL не могут содержать логин/пароль;
- минимум 5 секунд между циклами;
- максимум 20 targets;
- максимум 24 часа за один запуск;
- инфраструктуру не изменяет.

Пример 6-часовой проверки:

```bash
python3 scripts/soak-monitor.py \
  --url https://CONTROL/health/ready \
  --url https://CONTROL/api/v1/config/latest \
  --url https://INGRESS-1/__vpnx3/health \
  --url https://INGRESS-2/__vpnx3/health \
  --duration 21600 \
  --interval 15 \
  --output soak.jsonl \
  --fail-availability 99.9 \
  --fail-p95-ms 1000
```

Итог содержит:

- availability по каждому target;
- mean/p50/p95/p99/max latency;
- максимальную серию последовательных отказов;
- распределение ошибок.

Пороги для production SLA должны задаваться после фактических измерений, а не выбираться из документа.
