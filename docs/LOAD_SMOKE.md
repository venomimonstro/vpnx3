# Bounded load smoke

`scripts/load-smoke.py` — ограниченная нагрузочная проверка собственных VPNX3 HTTPS endpoints.

Она специально имеет жёсткие пределы:

- максимум 100 параллельных запросов;
- максимум 300 секунд;
- максимум 20 000 запросов;
- только явно перечисленные HTTPS URL;
- credentials в URL запрещены.

Это не стресс/DDoS-инструмент. Цель — после deploy измерить деградацию обычных endpoint под умеренной конкурентной нагрузкой.

## Базовая проверка Control Plane

```bash
python3 scripts/load-smoke.py \
  --url https://CONTROL/health/ready \
  --url https://CONTROL/api/v1/config/latest \
  --url https://CONTROL/api/v1/plans \
  --concurrency 10 \
  --duration 30 \
  --fail-error-rate 1 \
  --fail-p95-ms 800
```

## Browser ingress health

```bash
python3 scripts/load-smoke.py \
  --url https://INGRESS-1/__vpnx3/health \
  --url https://INGRESS-2/__vpnx3/health \
  --concurrency 20 \
  --duration 60 \
  --fail-error-rate 0.5 \
  --fail-p95-ms 500
```

Отчёт JSON содержит:

- число запросов;
- запросы/секунду;
- общий процент ошибок;
- mean/p50/p95/p99/max;
- HTTP status по каждому target;
- ошибки по каждому target.

Пороговые значения должны фиксироваться после измерений на реальной инфраструктуре. Первичный ориентир не является SLA.
