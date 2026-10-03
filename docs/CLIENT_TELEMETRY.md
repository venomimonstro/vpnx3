# Client telemetry

Клиентская диагностика хранится только в агрегированной дневной таблице.

Control Plane не сохраняет URL посещённых сайтов, DNS-запросы, IP назначения, содержимое трафика или индивидуальные telemetry events.

После проверки device signature событие сразу агрегируется по дню, платформе, версии приложения, типу события, версии подписанной конфигурации, worker node и типу сети.

Поддерживаемые события: connect_success, connect_failed, disconnect, recovered, stale_session_cleaned.

Device ID используется только для аутентификации запроса и replay sequence; в telemetry table он не записывается.
