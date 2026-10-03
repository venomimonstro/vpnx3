# Build Factory protocol

## Модель

Build Factory отделён от Control Plane.

Control Plane хранит только:

- release version;
- source commit SHA-1;
- заранее разрешённые build targets;
- очередь build jobs;
- состояние выполнения;
- метаданные будущих артефактов.

## Роль build_worker

Build worker подключается через стандартный одноразовый enrollment token с ролью `build_worker` и получает собственную Ed25519 identity.

Для build API используется отдельная монотонная `build_sequence`, поэтому heartbeat и build-команды не делят один replay counter.

## Claim

`POST /api/v1/build/claim`

Worker передаёт только список поддерживаемых целей. Сервер выбирает старейшую совместимую задачу через `FOR UPDATE SKIP LOCKED`. Две машины не могут получить одну queued-задачу.

## Complete

`POST /api/v1/build/jobs/{id}/complete`

Worker может сообщить только `succeeded` или `failed`. Он не может менять release, target или source commit.

## Запрет произвольных команд

Admin API принимает только:

- version;
- 40-символьный source commit;
- notes;
- allowlisted target.

Shell-команды, Gradle arguments, скрипты и пути из административного запроса не принимаются. Реальные build recipes зашиваются в версию build-worker.
