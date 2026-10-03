# Distributed probes

Спринт 5 начинает независимое наблюдение сети.

Probe является отдельной нодой с ролью `probe` и своей Ed25519 identity. Он:

1. регистрируется обычным одноразовым enrollment token для роли probe;
2. получает и криптографически проверяет Configuration Manifest;
3. проверяет публичные HTTPS `session_api` endpoints worker;
4. отправляет подписанный отчёт в Control Plane;
5. использует отдельную монотонную sequence для защиты отчётов от replay.

Control Plane хранит наблюдения и каждые 30 секунд пересчитывает `health_score` как долю успешных наблюдений за последние 5 минут.

На первом шаге probe проверяет HTTPS/TLS-доступность session API. UDP/WireGuard synthetic traffic будет добавлен отдельно, потому что UDP-порт нельзя качественно оценить обычным connect-check.
