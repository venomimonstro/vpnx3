# Audit integrity

`audit_log` защищён двумя уровнями:

1. UPDATE/DELETE запрещены trigger-ом;
2. каждая запись содержит `prev_hash` и `entry_hash`, образуя SHA-256 цепочку.

Insert сериализуется PostgreSQL advisory transaction lock, поэтому конкурентные административные операции не создают две разные «головы» цепочки.

`verify_audit_chain()` пересчитывает цепочку и входит в launch readiness. Несовпадение делает readiness `failed`.

Ограничение: пользователь PostgreSQL с полномочиями суперпользователя способен изменить сами функции/триггеры. Для повышенного уровня контроля audit stream следует дополнительно экспортировать во внешнее append-only/WORM хранилище или SIEM под отдельными credentials.
