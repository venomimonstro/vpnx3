# Admin API

Спринт 7 начинает административный интерфейс с серверных контрактов.

Добавлены защищённые RBAC endpoints:

- `GET /api/v1/admin/dashboard` — бизнес и технические KPI;
- `GET /api/v1/admin/users` — пользователи, устройства, активная подписка;
- `GET /api/v1/admin/users/{id}/devices`;
- `GET /api/v1/admin/payments`;
- `GET /api/v1/admin/audit`;
- `GET /api/v1/admin/incidents`;
- `POST /api/v1/admin/incidents`;
- `POST /api/v1/admin/incidents/{id}/resolve`.

Также миграция добавляет ранее отсутствовавшее разрешение `config.manage`, из-за которого endpoint публикации Configuration Manifest мог оставаться недоступным даже после появления его HTTP-маршрута.

Админский UI должен использовать только эти серверные API и не рассчитывать бизнес-показатели самостоятельно.
