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
- systemd installer worker;
- installer чистого Debian/Ubuntu сам создаёт WireGuard interface, gateway, forwarding и NAT;
- единый `install-worker-node.sh` поднимает Data Plane + Node Agent одним сценарием.

Не закрыто физически: сквозной smoke-test на реальном Linux-host и проверка трафика из внешней сети.

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

Дополнительно реализованы:
- упрощённый русскоязычный главный экран;
- основной сценарий «состояние защиты → Подключить»;
- тариф/устройства и системные VPN-настройки вынесены во вторичные разделы;
- закончившийся trial/доступ определяется до попытки соединения;
- после ошибки первого запуска можно повторить инициализацию без перезапуска приложения.

Следующие задачи:

1. физическая сборка APK/AAB на Android SDK build-host;
2. device/emulator smoke test;
3. проверка Always-on/lockdown поведения на реальных версиях Android;
4. финальная UX-полировка по результатам реального device-test.

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

Дополнительно реализованы:
- latency-aware routing по свежим независимым probe observations;
- агрегированная client telemetry;
- probe-only Access Lease с отдельной replay-sequence;
- signed `gateway_ipv4`;
- synthetic WireGuard data-plane probe: session creation → временный peer → handshake → ping gateway → session teardown;
- installer probe-ноды с CAP_NET_ADMIN/CAP_NET_RAW и необходимыми системными зависимостями.

Следующие задачи:

1. физически развернуть минимум 2–3 probes в разных сетях/регионах;
2. проверить synthetic WireGuard probe на реальных worker-хостах;
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

Дополнительно реализованы refund lifecycle, payment history API/UI, финансовая аналитика, payment abuse protection и подготовлена схема сохранённых payment methods/renewal attempts для безопасного auto-renew.

Следующие задачи:

1. sandbox e2e ЮKassa;
2. завершить server-side auto-renew execution и пользовательский opt-in/opt-out;
3. проверить recurring payment flow в тестовом магазине ЮKassa.

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

Дополнительно реализованы полноценная карточка пользователя (подписка, лимит устройств, trial, устройства, последние платежи), безопасная реактивация с проверкой device_limit и часть device-management UX без prompt-dialogs.

Дополнительно реализованы server-side search/filter пользователей и платежей, а также 30-дневная финансовая динамика поступлений/возвратов/чистого результата.

Дополнительно реализованы:
- полностью prompt-free критические операции админки;
- validated dialogs для enrollment/endpoints/тарифов/релизов/инцидентов/администраторов;
- launch readiness preflight с отдельными failed/warning/ok проверками для manifest, worker pool, probes, WireGuard data plane, Build Factory, release signing, платежей, artifact storage и browser ingress.

Следующие задачи: расширенный support tooling и физическая эксплуатационная проверка всех readiness-сигналов.

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

Дополнительно реализованы:
- verified Android build-host provisioning (JDK 17, Gradle, Android SDK с обязательным SHA-256);
- compileSdk 37 preflight;
- Android versionName/versionCode из Release;
- unit tests version mapping;
- stale build-worker watchdog;
- signed server-binary release targets;
- artifact storage abstraction с atomic local backend;
- hardened streaming/download deadlines и integrity checks.

Дополнительно реализован S3-compatible artifact backend с AWS Signature V4, обязательным HTTPS, SHA-256 verification, local spool, readiness и тем же Storage interface.

Дополнительно реализованы signed target `build_worker_darwin_arm64`, подготовка macOS/Apple Silicon Build Worker через launchd и локальное хранение Apple signing assets в macOS Keychain.

Следующие задачи: создать `apps/ios` и включить фиксированный Xcode archive/export recipe для `ios_ipa`.

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
4. физический smoke-test reconnect/failover на сетевых переключениях.

Дополнительно реализованы:
- one-minute browser maintenance loop;
- раннее обновление Proxy Lease;
- автоматическое переключение ingress по свежему signed manifest;
- ограничение proxy-auth retry loop;
- повторная выдача credential после 407;
- shared paid account Android/Chrome/Firefox через pairing code.


## Спринт 10 — эксплуатационная устойчивость и abuse protection: В РАБОТЕ

Реализовано:

- автоматический signed Configuration Manifest при старте;
- периодическое обновление manifest каждые 15 минут;
- немедленный refresh после circuit breaker;
- немедленный refresh после node lifecycle/endpoint mutations;
- retention старых config manifests;
- hashed/HMAC registration rate limiter;
- агрегация источника по IPv4 /24 и IPv6 /64;
- trusted proxy CIDR validation для X-Forwarded-For/X-Real-IP;
- один активный pairing code на устройство;
- очистка pairing codes;
- очистка registration rate buckets;
- удаление только полностью заброшенных anonymous trial accounts спустя 30 дней;
- admin device reactivation не может превысить plan.device_limit.

Следующие задачи:

Дополнительно реализованы:
- privacy-preserving daily security counters;
- dashboard блокировок регистраций/платежей;
- payment creation rate limit;
- детерминированная YooKassa idempotence для повторных кликов.

Следующие задачи:

Дополнительно реализованы:
- ручной production smoke-suite без CI;
- guarded failover drill с drain/restore и проверкой изменения signed manifest;
- bounded load smoke с p50/p95/p99, error rate и throughput;
- production failover drill по умолчанию заблокирован без явного подтверждения.

Следующие задачи:

1. прогнать smoke/failover/load проверки на реальной распределённой инфраструктуре;
2. зафиксировать эксплуатационные пороги после измерений;
3. провести длительный soak-test и проверить восстановление после реальных сетевых отказов.


## Спринт 11 — коммерческий preflight и операционная панель: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Реализовано:

- отдельный `GET /api/v1/admin/readiness`;
- проверка свежести signed Configuration Manifest;
- проверка active/routable worker pool;
- проверка active browser ingress;
- проверка свежих независимых probe observations;
- проверка успешного synthetic WireGuard data-plane observation;
- проверка согласованности Build Factory queue/active workers;
- проверка release signing/publication;
- проверка конфигурации платёжного адаптера;
- readiness artifact storage;
- отдельный экран «Готовность запуска» с уровнями ok/warning/failed.

Дополнительно добавлены ручные production smoke, failover drill и bounded load smoke, использующие readiness как единый контрольный контур.

Следующий этап: прогон preflight/smoke/failover/load на реальной распределённой инфраструктуре и фиксация эксплуатационных порогов по фактической нагрузке.


## Спринт 12 — личный ключ устройства: КОДОВАЯ ОСНОВА В РАБОТЕ

Цель: отдельный персональный WireGuard-ключ для личного использования, приватная часть которого недоступна Control Plane и не хранится в серверной БД.

Реализовано:

- отдельный PersonalTunnelKeyStore;
- приватный WireGuard-ключ создаётся только на Android-устройстве;
- ключ шифруется AES-256-GCM ключом Android Keystore;
- StrongBox запрашивается автоматически на поддерживаемых устройствах;
- fallback на аппаратный Android Keystore, затем на стандартный Keystore;
- Android backup приложения уже полностью отключён;
- отдельный локальный переключатель «Личный ключ»;
- основной сервер получает только публичный WireGuard-ключ, необходимый для создания peer;
- локальная ротация и удаление ключа;
- режим не синхронизируется через Control Plane;
- переключение/ротация запрещены во время активного VPN-соединения.

Модель угроз:

- компрометация PostgreSQL или Control Plane не раскрывает приватный личный ключ;
- server-side backup не содержит приватного ключа;
- потерянный/удалённый device-only ключ восстановить нельзя;
- VPN-инфраструктура по-прежнему видит факт сессии, публичный WireGuard-ключ и технические метаданные, необходимые для работы сервиса.

Следующие задачи:

1. физически проверить StrongBox/hardware-backed ветки минимум на двух Android-устройствах;
2. добавить подтверждение биометрией как опциональный режим для операций ротации/использования личного ключа;
3. проверить поведение после переустановки приложения и переноса данных между устройствами;
4. реализовать эквивалентный device-only key container для iOS Keychain/Secure Enclave, где применимо.
