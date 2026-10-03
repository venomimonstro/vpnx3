# VPNX3 — план разработки и фактический статус

## Правила разработки

- основная ветка: `main`;
- GitHub Actions/CI не используются;
- секреты не помещаются в Git;
- каждый спринт должен оставлять проект в согласованном состоянии;
- сетевой контур отделён от бизнес-логики;
- статус ниже отражает фактически добавленный код.

## Спринт 0 — фундамент Control Plane: ЗАВЕРШЁН

PostgreSQL, миграции, health/readiness, RBAC, административная авторизация, Argon2id, хешированные сессии, bootstrap owner, append-only audit, защитные HTTP-заголовки и контейнерное окружение.

## Спринт 1 — управление нодами: ЗАВЕРШЁН

Одноразовое подключение, индивидуальная Ed25519-идентичность нод, Node Agent, подписанный heartbeat, replay protection, состояния нод, publish/drain/maintenance/quarantine/retire, история, heartbeat timeout и безопасный installer.

## Спринт 2 — пользовательский доступ / Data Plane: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Реализованы Access Lease, offline-проверка права, Worker, транспортный контракт, IPAM, WireGuard через wgctrl, lifecycle сессий, восстановление после рестарта, live session count через Node Agent и управляемые HTTPS/UDP endpoints нод.

Остаётся инфраструктурная проверка на реальном Linux-host: `wg0`, forwarding/NAT и сквозной трафик. Без такого хоста нельзя достоверно заявить, что физический e2e-туннель протестирован.

## Спринт 3 — подписанная конфигурация: ОСНОВА ЗАВЕРШЕНА

Ed25519 Configuration Manifest, точные подписанные payload bytes, versioning, expiry, rollback protection, Last Known Good и публикация активных ingress/worker endpoints.

## Спринт 4 — Android MVP: В РАБОТЕ

Реализовано:

- нативный Kotlin/Jetpack Compose клиент;
- Android Keystore identity ECDSA P-256;
- анонимная регистрация;
- подписанные device-запросы;
- Access Lease;
- pinning Configuration public key на build-time;
- проверка Ed25519 Configuration Manifest;
- Last Known Good configuration;
- выбор worker из подписанного манифеста;
- отдельная Curve25519 WireGuard key pair;
- шифрование WireGuard private key AES-GCM ключом Android Keystore;
- HTTPS создание worker session;
- сверка WireGuard endpoint с подписанным манифестом;
- официальный WireGuard Android tunnel library / GoBackend;
- состояния DISCONNECTED / CONNECTING / CONNECTED / ERROR;
- отключение с удалением worker peer.

Следующие задачи Android:

1. foreground/always-on UX и корректное восстановление процесса;
2. DNS и MTU как подписанная политика конфигурации;
3. retry/failover на следующий worker;
4. диагностика и измерение времени подключения;
5. kill-switch UX;
6. локальные тесты Android и сборка APK.

## Следом

### Спринт 5 — распределённое наблюдение
Probes, региональная доступность, клиентская агрегированная телеметрия, health score и circuit breakers.

### Спринт 6 — биллинг: В РАБОТЕ

Уже реализованы версионируемые тарифы, публичный read-only каталог продаваемых тарифов, admin API тарифов, provider-neutral billing core, таблицы payments/billing_events, SHA-256 payload audit, идемпотентность provider events и транзакционное создание/продление подписки. Версионирование одного plan code защищено PostgreSQL advisory lock от конкурентной гонки.

Следующий шаг — конкретный payment provider adapter с обязательной проверкой подписи webhook, затем refund/cancel lifecycle и финансовая аналитика.

### Спринт 7 — административный интерфейс
Сеть, устойчивость, пользователи, тарифы, платежи, релизы, инциденты.

### Спринт 8 — Build Factory
Изолированные сборки APK/AAB и расширений, отдельная подпись и история выпусков.

Полная долгосрочная архитектура: `docs/CONCEPT.md`.
