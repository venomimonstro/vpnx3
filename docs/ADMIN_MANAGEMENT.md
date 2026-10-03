# Administrator management

Администраторы управляются существующими таблицами roles / permissions.

## Защиты

- Нельзя менять собственные роли через административный API.
- Нельзя отключить собственную учётную запись.
- Нельзя удалить роль owner у последнего активного владельца.
- Нельзя отключить последнего активного владельца.
- При status=disabled все активные admin_sessions пользователя транзакционно отзываются.
- Пароль нового администратора проходит тот же Argon2id policy и minimum length, что bootstrap owner.
- Все create/role/status операции записываются в append-only audit log.

Поддерживаемые роли уже определены миграциями: owner, security_admin, infrastructure, finance, support, analyst, read_only.
