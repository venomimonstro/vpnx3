# Admin UI

Административная панель встроена в Control Plane и доступна по `/admin/`.

Отдельный Node/npm runtime не используется. HTML/CSS/JS компилируются в Go binary через `embed`, поэтому версия UI всегда соответствует версии API.

## Безопасность

- нет внешних CDN, шрифтов или аналитики;
- Content-Security-Policy разрешает только ресурсы того же origin;
- frame-ancestors запрещён;
- камера, микрофон, геолокация и браузерный Payment API отключены через Permissions-Policy;
- bearer token хранится только в `sessionStorage`, а не localStorage;
- пользовательские данные вставляются через DOM `textContent`, не через HTML-шаблонизацию;
- UI скрывает недоступные разделы по permissions, но серверный RBAC остаётся обязательным источником авторизации.

## Разделы

- Обзор: пользователи, устройства, подписки, ноды, сессии, выручка, probe success.
- Сеть: ноды, статусы, health, circuit breaker, endpoints, enrollment token, публикация config.
- Наблюдение: probe results и latency.
- Пользователи.
- Тарифы и платежи.
- Инциденты.
- Audit log.

Интерфейс является операторской панелью, а не местом хранения бизнес-логики.
