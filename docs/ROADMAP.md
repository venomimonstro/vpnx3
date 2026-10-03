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
- sequence durability: номер резервируется до сетевого запроса;
- process recovery через реальное состояние WireGuard GoBackend;
- cleanup stale worker session после process death;
- агрегированная подписанная client telemetry без истории трафика;
- server-priced billing flow;
- account/subscription status;
- multi-device pairing в пределах тарифа;
- переход в системные VPN settings для Always-on VPN / kill switch.

Следующие задачи:

1. физическая сборка APK/AAB на Android SDK build-host;
2. device/emulator smoke test;
3. проверка Always-on/lockdown поведения на реальных версиях Android;
4. UX-полировка и локализация.

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

Дополнительно реализованы latency-aware routing по свежим независимым probe observations и агрегированная client telemetry.

Следующие задачи:

1. физически развернуть минимум 2–3 probes в разных сетях/регионах;
2. synthetic WireGuard data-plane probe;
3. проверить circuit-breaker/failover на реальной распределённой сети.

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

## Спринт 7 — административный интерфейс: ОСНОВА РЕАЛИЗОВАНА

Реализованы:

- встроенная в Go binary административная панель без отдельного Node/npm runtime;
- login/logout и RBAC-адаптивная навигация;
- persistent login throttling;
- dashboard KPI;
- ноды, lifecycle actions, endpoints и enrollment tokens;
- публикация подписанной сетевой конфигурации;
- probes/circuit breaker observations;
- пользователи;
- versioned plans и история платежей;
- incidents;
- append-only audit log;
- releases/build jobs/artifacts.

Дополнительно реализованы управление администраторами/ролями, отзыв устройств, account device limits, клиентская диагностика, release management и финансовые показатели/refund ledger.

Следующие задачи: фильтры/поиск, полноценные карточки пользователей, графики без prompt-dialog UX и support tooling.

## Спринт 8 — Build Factory: В РАБОТЕ

Реализованы:

- роль build_worker;
- подписанный build protocol с отдельным replay sequence;
- очередь build jobs через FOR UPDATE SKIP LOCKED;
- allowlisted build targets;
- fixed recipes без sh -c;
- exact Git commit checkout и verification;
- Android APK/AAB recipes;
- release signing secrets только на build worker;
- потоковая artifact upload с подписанным SHA-256;
- persistent artifact volume;
- safe retry failed jobs;
- release lifecycle ready/published/withdrawn;
- административное скачивание artifacts;
- отдельный Ed25519 Release Signing Key;
- подписанный публичный release manifest;
- Android проверяет release metadata локально.

Дополнительно реализованы Chrome/Firefox extension sources и recipes, server binary targets, browser ingress binary, version injection, verified ingress installer, artifact retention и signed release distribution.

Следующие задачи: готовый Android SDK build-host, macOS worker + Xcode для iOS и S3-compatible artifact adapter.

## Обязательные проверки вне текущей GitHub-среды

Эта среда не даёт полноценный Linux WireGuard host, Android SDK/emulator и платёжный sandbox. Поэтому нельзя честно считать физически протестированными:

- реальный VPN data traffic;
- APK/AAB build и Android runtime;
- YooKassa sandbox payment/webhook.

Кодовые контуры для этих тестов реализованы; следующий эксплуатационный этап должен прогнать именно эти проверки.


## Спринт 9 — Browser Transport: В РАБОТЕ

Реализовано:

- отдельный scoped Proxy Lease с browser_proxy scope;
- HTTPS CONNECT ingress;
- SSRF-защита private/loopback/link-local/CGNAT и ограничение ports 80/443;
- Chrome и Firefox extensions;
- ECDSA P-256 device identity;
- signed Configuration Manifest verification и rollback protection;
- proxy auth короткоживущим signed credential;
- ingress probing/circuit breaker;
- latency-aware ingress routing;
- shared paid account через pairing code;
- build recipes и version injection;
- verified systemd installer ingress-ноды.

Следующие задачи:

1. физический smoke-test Chrome/Firefox;
2. store-specific packaging/signing;
3. проверить auth callback compatibility на актуальных Chrome/Firefox;
4. UX reconnect/failover на сетевых переключениях.
