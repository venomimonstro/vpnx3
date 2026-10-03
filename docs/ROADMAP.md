# VPNX3 — план разработки и фактический статус

## Правила разработки

- основная ветка: `main`;
- GitHub Actions/CI не используются;
- секреты не помещаются в Git;
- каждый спринт должен оставлять проект в согласованном состоянии;
- сетевой контур отделён от бизнес-логики;
- статус ниже отражает фактически добавленный код.

## Спринт 0 — фундамент Control Plane: ЗАВЕРШЁН

PostgreSQL, версионные миграции, health/readiness, RBAC, административная авторизация, Argon2id, хешированные сессии, bootstrap owner, append-only audit, request ID, защитные HTTP-заголовки и контейнерное окружение.

## Спринт 1 — управление нодами: ЗАВЕРШЁН

Одноразовое подключение, индивидуальная Ed25519 identity нод, Node Agent, подписанный heartbeat, replay protection, state machine, publish/drain/maintenance/quarantine/retire, история состояний, heartbeat timeout и systemd installer с SHA-256-проверкой бинарника.

## Спринт 2 — Data Plane: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Реализовано:

- анонимная регистрация устройства;
- Android Keystore identity ECDSA P-256;
- пробный доступ и подписанный Access Lease;
- привязка Access Lease к WireGuard public key;
- offline-авторизация worker;
- transport adapter interface;
- IPAM;
- WireGuard через wgctrl;
- lifecycle сессий;
- persistence/recovery worker;
- live session count в heartbeat;
- managed HTTPS/UDP node endpoints;
- systemd installer worker.

Не закрыто физически: реальный Linux-host с `wg0`, forwarding/NAT и сквозной тест трафика.

## Спринт 3 — подписанная конфигурация: ОСНОВА ЗАВЕРШЕНА

Ed25519 Configuration Manifest, exact payload bytes, version/expiry, rollback protection, Last Known Good, worker/ingress endpoints, подписанная DNS/MTU/keepalive policy.

## Спринт 4 — Android MVP: В РАБОТЕ

Реализовано:

- Kotlin + Jetpack Compose;
- Android Keystore device identity;
- отдельная WireGuard Curve25519 key pair;
- шифрование WireGuard private key AES-GCM ключом Android Keystore;
- signed config verification;
- Last Known Good;
- worker routing/failover;
- Access Lease;
- HTTPS worker session;
- endpoint verification;
- официальный WireGuard Android GoBackend;
- CONNECTING/CONNECTED/DISCONNECTED/ERROR;
- disconnect с удалением peer;
- sequence durability: номер резервируется до сетевого запроса.

Следующие задачи:

1. сборка APK на Android SDK/Gradle окружении;
2. device/emulator smoke test;
3. process/always-on recovery;
4. foreground UX/notification;
5. connection diagnostics/telemetry;
6. kill-switch UX.

## Спринт 5 — распределённое наблюдение: В РАБОТЕ

Реализовано:

- независимые probe-ноды;
- Ed25519 probe identity;
- signed probe reports;
- persistent replay sequence;
- HTTPS/TLS checks;
- история probe results;
- health score;
- multi-probe circuit breaker;
- автоматическое восстановление только circuit-breaker-degraded нод.

Следующие задачи:

1. минимум 2–3 probes в разных сетях/регионах;
2. synthetic WireGuard data-plane probe;
3. latency-aware routing;
4. client aggregate telemetry.

## Спринт 6 — биллинг: В РАБОТЕ

Реализовано:

- versioned plans;
- публичный каталог тарифов;
- admin API тарифов;
- payments/billing_events;
- provider-neutral Billing Core;
- webhook idempotency;
- server-side plan amount/currency verification;
- защита от двойного entitlement grant;
- YooKassa adapter;
- YooKassa payment creation;
- server-side verification webhook через повторный GET payment API;
- транзакционное создание/продление subscription.

Следующие задачи:

1. sandbox e2e ЮKassa;
2. refund lifecycle;
3. payment history API/UI;
4. auto-renew;
5. финансовая аналитика.

## Спринт 7 — административный интерфейс: СЛЕДУЮЩИЙ КРУПНЫЙ

Нужны: dashboard, сеть/ноды/endpoints, probes/circuit breakers, пользователи/устройства, тарифы/платежи, incidents, releases и audit.

## Спринт 8 — Build Factory

Изолированные сборки APK/AAB и расширений, отдельная подпись, история выпусков и безопасная публикация артефактов.

## Обязательные проверки вне текущей GitHub-среды

Эта среда не даёт полноценный Linux WireGuard host, Android SDK/emulator и платёжный sandbox. Поэтому нельзя честно считать физически протестированными:

- реальный VPN data traffic;
- APK/AAB build и Android runtime;
- YooKassa sandbox payment/webhook.

Кодовые контуры для этих тестов реализованы; следующий эксплуатационный этап должен прогнать именно эти проверки.
