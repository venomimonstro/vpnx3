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

## Спринт 4 — Android MVP: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

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

## Спринт 5 — распределённое наблюдение: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

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

## Спринт 6 — биллинг: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

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

Код дополнительно включает explicit opt-in/opt-out auto-renew, сохранение только provider payment-method ID, идемпотентный scheduler, provider reconciliation зависших попыток, максимум три отказа на цикл и автоматическое отключение автопродления после серии ошибок.

Физическая приёмка:
1. sandbox e2e ЮKassa;
2. recurring payment flow в тестовом магазине ЮKassa;
3. webhook/reconciliation сценарии с реальными provider statuses.

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

## Спринт 8 — Build Factory: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

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

Дополнительно реализованы signed target `build_worker_darwin_arm64`, macOS/Apple Silicon Build Worker через launchd, SwiftUI iOS-клиент, Packet Tunnel Extension, pinned WireGuardKit revision, фиксированный Xcode archive/export recipe для `ios_ipa`, shared device-only Keychain для app/extension и обязательная проверка подписанного IPA перед публикацией.

Физическая приёмка: реальная сборка/подпись на macOS с Apple Developer Team, установка IPA/TestFlight и проверка provisioning entitlement.

## Обязательные проверки вне текущей GitHub-среды

Эта среда не даёт полноценный Linux WireGuard host, Android SDK/emulator и платёжный sandbox. Поэтому нельзя честно считать физически протестированными:

- реальный VPN data traffic;
- APK/AAB build и Android runtime;
- YooKassa sandbox payment/webhook.

Кодовые контуры для этих тестов реализованы; следующий эксплуатационный этап должен прогнать именно эти проверки.


## Спринт 9 — Browser Transport: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

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


## Спринт 10 — эксплуатационная устойчивость и abuse protection: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

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
- read-only soak monitor до 24 часов с availability и сериями отказов;
- production failover drill по умолчанию заблокирован без явного подтверждения.

Следующие задачи:

1. прогнать smoke/failover/load проверки на реальной распределённой инфраструктуре;
2. зафиксировать эксплуатационные пороги после измерений;
3. прогнать длительный soak-monitor и проверить восстановление после реальных сетевых отказов.


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


## Спринт 12 — личный ключ устройства: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

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
4. физически проверить iOS shared ThisDeviceOnly Keychain между приложением и Packet Tunnel Extension.

Дополнительно реализован iOS device-only WireGuard key: приватная часть хранится в shared ThisDeviceOnly Keychain access group, доступном только подписанным app/PacketTunnel targets; в NETunnelProvider preferences приватный ключ не сериализуется.


## Спринт 13 — Backup / Disaster Recovery: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Реализовано:

- PostgreSQL custom-format backup;
- обязательное шифрование backup bundle через age;
- SHA-256 manifest и проверка размера;
- опциональный backup local artifact volume;
- отдельный verify-backup;
- restore drill только в явно подтверждённую изолированную БД;
- защита от случайного restore в основную БД по умолчанию;
- systemd timer для ежедневного backup;
- root-only backup environment;
- health-marker обновляется только после успешного backup;
- launch readiness контролирует свежесть backup;
- production release gate блокирует публикацию при отсутствующем/просроченном backup;
- документирован отдельный secret escrow: signing/payment/storage credentials не кладутся в database backup.

Физическая приёмка:

1. настроить off-provider backup destination;
2. выполнить первый encrypted backup;
3. выполнить restore drill на отдельном PostgreSQL;
4. проверить RPO/RTO по фактическому объёму БД;
5. повторять restore drill минимум ежемесячно.

## Спринт 14 — Security Hardening: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Реализовано:

- Configuration / Access / Release Ed25519 seeds валидируются и обязаны быть разными;
- production требует Release Signing Key;
- production запрещает debug logging;
- production admin session TTL не может превышать 24 часа;
- admin session binding: user-agent или усиленный network режим;
- при session-context mismatch bearer автоматически отзывается и событие попадает в audit;
- remote production PostgreSQL запрещён без TLS sslmode require/verify-ca/verify-full;
- audit_log остаётся append-only;
- поверх append-only добавлена SHA-256 hash chain;
- verify_audit_chain входит в launch readiness;
- iOS WireGuard private keys не сериализуются в NETunnelProvider preferences;
- app и PacketTunnel используют общий ThisDeviceOnly Keychain access group;
- iOS release validator проверяет entitlement и shared keychain group;
- migrator fail-closed при duplicate migration versions.

Ограничение: DB-superuser теоретически способен менять сами audit functions/triggers. Для более высокого уровня контроля audit stream следует экспортировать во внешнее WORM/SIEM.

## Спринт 15 — Production Readiness: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Реализовано:

- launch readiness как единый операционный preflight;
- actionable operational issue queue;
- production release gate;
- production publish блокируется при критическом состоянии manifest/worker/probes/data-plane/audit/backup/artifact storage/release signing/payment adapter;
- browser release требует active ingress;
- manual production-release-check с машинными exit codes;
- append-only release publication attestation со snapshot gate signals;
- local-quality-gate без GitHub Actions/CI;
- migration-smoke на временном PostgreSQL 16;
- migration smoke проверяет audit chain/append-only и критические schema invariants;
- historical duplicate migration 000019 устранён;
- новая 000027 repair migration гарантирует конечную схему и для ранее развернутых БД, независимо от того, какой старый 000019 был записан;
- migrator теперь запрещает любые будущие duplicate migration versions.

Физическая production-приёмка, которую нельзя честно объявить выполненной из GitHub-среды:

1. Linux WireGuard e2e на реальных worker-hosts;
2. минимум 2–3 независимых probe;
3. Chrome/Firefox smoke на актуальных браузерах;
4. Android APK/AAB build + real-device test;
5. iOS macOS build + TestFlight/real-device NetworkExtension test;
6. YooKassa sandbox + recurring payment e2e;
7. encrypted backup + restore drill;
8. failover drill;
9. bounded load smoke;
10. длительный soak monitor;
11. после успешных физических проверок — первый production release через enforced release gate.

## Текущий итог

Кодовый план Sprint 0–17 реализован. Оставшиеся пункты — не «ещё написать код», а физическая приёмка тех частей, которые требуют реальных ОС, сетей, платёжного sandbox, Apple/Android signing infrastructure и нескольких независимых серверов.

Отдельно: никакой документ не должен утверждать «100% стабильность». Цель проекта — отсутствие single-node global outage, fail-closed security boundaries, воспроизводимые релизы, наблюдаемость, controlled degradation и измеряемые SLO.


## Спринт 16 — рост и реферальная программа: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Реализовано:

- индивидуальный referral code пользователя;
- claim только в ограниченном окне до первой оплаты;
- запрет self-referral и повторного claim;
- бонус приглашённому применяется к trial/подписке;
- бонус пригласившему выдаётся только после квалифицирующей успешной оплаты;
- refund qualifying payment отзывает соответствующую reward;
- target device фиксируется для trial reward;
- Android/iOS: просмотр кода, claim и статус наград;
- Chrome/Firefox: просмотр своего кода, применение кода и статус наград через подписанный device API;
- dashboard: claimed/qualified/reward days;
- отдельный экран админки с 30-дневной воронкой, конверсией, отзывами наград и последними redemption;
- карточка поддержки пользователя показывает реферальное состояние.

Физическая приёмка: проверить полный цикл referral → первая оплата → reward → refund/revoke на sandbox-данных.

## Спринт 17 — поддержка и диагностика пользователя: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Карточка пользователя объединяет:

- подписку/trial и лимит устройств;
- все устройства и revoke/reactivate;
- последние платежи;
- последние renewal attempts и provider payment id;
- реферальный статус и выданные/ожидающие/отозванные награды;
- support signals: нет доступа, нет активного устройства, ошибки оплаты, ошибки автопродления;
- последние audit events пользователя и его устройств.

Цель: типовая диагностика обращения должна выполняться из одной карточки без ручного SQL.


## Спринт 18 — внешний журнал безопасности и уведомления: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: критические административные и security-события должны иметь независимую от основной БД внешнюю копию.

Реализовано:

- transactional security_event_outbox, заполняемый после append-only audit_log;
- очередь не участвует в критическом пути клиентских запросов;
- конкурентная доставка через FOR UPDATE SKIP LOCKED;
- отдельный Ed25519 signing key, не совпадающий с Configuration/Access/Release keys;
- signed envelope содержит audit chain prev_hash/entry_hash;
- внешний endpoint только HTTPS и без credentials в URL;
- ограниченный batch;
- retry с экспоненциальной задержкой;
- успешная доставка фиксируется отдельно, исходный audit_log не меняется.

Следующие задачи:

Дополнительно реализованы:
- readiness pending/oldest/max-attempts;
- пороги warning/failed по возрасту очереди;
- локальный loopback-only test receiver с Ed25519 verification и fsync NDJSON;
- runbook проверки signed envelope.

Дополнительно реализованы:
- transactional incident notification outbox на уровне БД;
- generic HTTPS webhook без сторонних SDK;
- HMAC-SHA256 подпись каждого exact JSON payload;
- notification retry/backoff независимо от Control Plane request path;
- уведомления создаются при insert/update/resolved incidents независимо от источника;
- readiness контролирует backlog/возраст/повторные ошибки уведомлений.

Дополнительно реализованы:
- system monitor с debounce: инцидент открывается после 3 последовательных плохих наблюдений и закрывается после 2 здоровых;
- системные incidents для worker pool, probes, synthetic WireGuard, Build Factory, audit chain, backup, security export и notification backlog;
- состояние открытого системного incident хранится в БД и переживает перезапуск Control Plane;
- автоматическое recovery закрывает incident и через outbox отправляет resolved notification.

Физическая приёмка:
1. подключить независимый WORM/SIEM endpoint;
2. подключить рабочий HTTPS webhook уведомлений;
3. искусственно вызвать/восстановить каждый системный fault и проверить open → notify → recover → resolved.


## Спринт 18 — внешний аудит и независимый след безопасности: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: локальная audit hash-chain не должна быть единственной копией критического журнала.

Реализовано:

- PostgreSQL trigger автоматически ставит каждую новую audit-запись в security outbox;
- доставка не блокирует основной HTTP-запрос;
- несколько Control Plane экземпляров делят очередь через FOR UPDATE SKIP LOCKED;
- delivery lease защищает от параллельной повторной отправки;
- наружу передаются audit_id, prev_hash, entry_hash и минимальные метаданные;
- before_state/after_state и source IP по умолчанию не экспортируются;
- payload подписывается отдельным Ed25519 signing key;
- receiver хранит только публичный ключ, дедуплицирует по audit_id и проверяет непрерывность hash-chain;
- экспоненциальные retry;
- после 10 безуспешных попыток событие переходит в dead-letter;
- launch readiness контролирует backlog, возраст очереди и dead-letter;
- отдельное RBAC-разрешение security.export.manage;
- owner/security-admin могут безопасно requeue dead-letter без ручного SQL;
- requeue записывается в audit_log;
- экран аудита показывает состояние внешней доставки;
- configuration fail-closed: URL только HTTPS, URL/secret задаются парой, secret минимум 32 символа.

Физическая приёмка:

1. подключить независимый WORM/SIEM/receiver вне основного PostgreSQL/Control Plane;
2. проверить Ed25519 verification и дедупликацию;
3. искусственно отключить receiver и проверить retry/backlog/dead-letter;
4. восстановить receiver и проверить непрерывность prev_hash → entry_hash;
5. хранить receiver credentials отдельно от database backup.


## Спринт 18 — управление версиями клиентов: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: безопасно раскатывать клиентские релизы и иметь аварийный механизм остановки несовместимой версии без ручного SQL и без неподписанных remote flags.

Реализовано:

- отдельная release policy для Android APK, iOS IPA, Chrome и Firefox;
- minimum supported version;
- recommended version;
- staged rollout 0–100%;
- список явно заблокированных версий;
- пользовательское сообщение;
- append-only история изменения политики;
- публичная политика подписывается отдельным Release Signing Key;
- Android проверяет подпись policy и блокирует только новую VPN-сессию при minimum/blocked;
- iOS проверяет signed policy и не рвёт уже активный tunnel;
- Chrome/Firefox проверяют signed policy и не сбрасывают активный proxy в DIRECT;
- rollout-когорта детерминирована по device ID + target + recommended version;
- браузерные пакеты теперь обязаны содержать pinned Release Signing Key;
- административная панель управляет политикой через структурированную форму;
- сервер запрещает политику, которая блокирует клиентов без реально опубликованного recommended artifact;
- minimum version не может быть выше recommended version.

Безопасная последовательность массовой раскатки:

1. опубликовать новый release artifact;
2. поставить recommended version и rollout 5–10%;
3. наблюдать crash/connect/payment/support метрики;
4. увеличить rollout 25% → 50% → 100%;
5. minimum supported version повышать только после устойчивого 100% rollout;
6. blocked version использовать только для подтверждённой критической несовместимости/уязвимости.

Физическая приёмка:

- проверить policy на реальных Android/iOS/Chrome/Firefox сборках;
- проверить сохранение активной сессии при появлении blocked policy;
- проверить staged cohort между перезапусками;
- проверить обновление через реальные store/direct distribution каналы.


## Спринт 19 — ёмкость и концентрационный риск сети: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: сеть должна быть не только доступной, но и иметь измеряемый запас ресурсов и отсутствие опасной зависимости от одного инфраструктурного сегмента.

Реализовано:

- агрегированная configured worker capacity;
- текущие active sessions и utilization/headroom;
- обнаружение active worker без capacity_sessions;
- breakdown capacity/sessions по VPS-провайдеру;
- breakdown capacity/sessions по стране;
- доля крупнейшего провайдера и крупнейшей страны;
- отдельный admin endpoint и экран «Риск сети»;
- readiness warning при >=70% utilization;
- readiness failed при >=85% utilization;
- provider concentration warning при >=50% capacity;
- provider concentration failed при >=70% capacity;
- debounced system incident при критической загрузке;
- debounced system incident при критической зависимости от одного провайдера.

Физическая приёмка:

1. заполнить capacity_sessions и provider/country для production worker;
2. сверить capacity с реальными нагрузочными измерениями;
3. добиться распределения, при котором один провайдер не несёт критическую долю;
4. проверить incident при искусственном снижении доступной capacity.


## Спринт 19 — самообслуживание устройств: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: пользователь самостоятельно управляет устройствами аккаунта без обращения в поддержку.

Реализовано:

- signed client API списка устройств;
- сервер определяет user_id только по подписанному текущему device identity;
- клиент не может получить список чужого аккаунта;
- текущее устройство явно помечается;
- текущее устройство нельзя случайно отозвать через self-service endpoint;
- можно отозвать только другое active устройство своего аккаунта;
- pairing-коды, созданные отозванным устройством, инвалидируются;
- действие записывается в audit log;
- Android показывает устройства и позволяет отключить другое;
- iOS показывает устройства и позволяет отключить другое;
- Chrome/Firefox показывают устройства и позволяют отключить другое.

Ограничение текущего этапа:

- новые запросы отозванного device identity блокируются сразу;
- уже выданный offline Access Lease действует до expiry, поэтому для быстрого закрытия уже активной worker-сессии нужен отдельный подписанный revocation feed.

Следующий Sprint 20 закрывает именно быстрое распространение revoke на worker-ноды без превращения Control Plane в обязательную зависимость каждой VPN-сессии.


## Спринт 20 — быстрое распространение отзыва устройств: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: отзыв устройства должен быстро закрывать уже выданные offline Access/Proxy Lease, но Control Plane не должен становиться обязательной зависимостью каждого пакета или каждой новой VPN-сессии.

Реализовано:

- versioned signed revocation snapshot на Access Signing Key;
- privacy-minimal device hash вместо raw device id в feed;
- rollback protection по монотонной версии;
- несколько revocation sources: Control Plane + config mirrors;
- локальный подписанный LKG snapshot на worker/ingress;
- ограниченный LKG grace, максимум 2 часа;
- сразу после expiry узел становится degraded/unhealthy и исключается из новых маршрутов;
- до hard-expiry допускается bounded LKG, чтобы краткий outage Control Plane не создавал глобальный обрыв;
- после hard-expiry новые VPN/proxy сессии fail-closed;
- после hard-expiry worker закрывает активные VPN sessions;
- после hard-expiry browser ingress закрывает активные proxy connections;
- новый snapshot немедленно закрывает сессии отозванных устройств;
- worker/ingress не запускаются без доверенного revocation source или пригодного signed LKG;
- config mirror проверяет подпись/версию/expiry и зеркалирует тот же единый revocation format;
- старый дублирующий internal/revocations feed удалён;
- installer worker/ingress явно задаёт revocation poll и LKG grace;
- health endpoint worker/ingress отражает revocation freshness/usable state;
- unit tests покрывают rollback, LKG boundaries, active-session revoke и CloseAll.

Физическая приёмка:

1. открыть VPN/Proxy сессию;
2. отозвать устройство из другого активного устройства;
3. проверить, что worker/ingress получают новую версию snapshot;
4. измерить фактическое время до закрытия active session;
5. отключить Control Plane, оставить mirror и проверить продолжение обновлений;
6. отключить все sources, проверить degraded → hard-expiry → fail-closed;
7. восстановить source и проверить автоматическое возвращение без ручной очистки state.


## Спринт 21 — ротация криптографических ключей: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: Configuration/Access/Release signing keys должны заменяться без массового ручного обновления клиентов и без хранения долгоживущего root private key на production-инфраструктуре.

Реализовано:

- offline Ed25519 Root of Trust; root seed не размещается на Control Plane, worker, ingress, mirror или клиентах;
- root-signed versioned trust bundle;
- отдельные purpose: config, access, release;
- состояния ключей active / next / retired;
- монотонная версия bundle и rollback protection;
- ограниченный срок действия bundle;
- exactly-one active config/access signer;
- Control Plane в production fail-closed проверяет root-signed bundle при старте;
- active runtime signer обязан совпадать с ключом, разрешённым bundle;
- launch readiness предупреждает менее чем за 7 дней до expiry и становится failed после expiry;
- Config Mirror проверяет root bundle и зеркалирует trust/config/revocations;
- Android хранит pinned root, signed bundle LKG и проверяет config/release через active+retired keyring;
- iOS хранит pinned root, device-only trust LKG и проверяет config/release через active+retired keyring;
- Chrome/Firefox хранят signed trust bundle в local storage и используют active+retired config/release keys;
- worker и browser ingress получают access/revocation/proxy keyring из root-signed bundle;
- worker/ingress обновляют keyring live без рестарта;
- worker/ingress имеют persisted signed LKG trust bundle;
- после trust bundle expiry новые lease fail-closed;
- installers worker/ingress принимают trust root и optional trust mirrors;
- legacy single-key режим сохранён только для миграционного перехода;
- unit test покрывает overlap old+new → удаление retired → fail-closed после expiry.

Безопасная последовательность ротации:

1. создать новый operational key;
2. опубликовать его как next в bundle версии N+1;
3. дождаться распространения bundle по клиентам/worker/ingress/mirror;
4. выпустить bundle N+2: новый ключ active, предыдущий retired;
5. переключить runtime signer;
6. выдержать bounded compatibility window;
7. выпустить bundle N+3 без предыдущего retired key;
8. проверить readiness и telemetry отказов подписи.

Физическая приёмка:

1. выполнить полную ротацию config key на staging без обновления приложений;
2. выполнить access key rotation при активных VPN/proxy sessions;
3. подтвердить, что старые lease работают только в overlap;
4. удалить retired key и подтвердить отказ старой подписи;
5. искусственно просрочить test bundle и подтвердить fail-closed;
6. отключить Control Plane и проверить получение trust bundle через Config Mirror.


## Спринт 22 — безопасное обновление серверных компонентов: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: worker, ingress, probe, config mirror и node agent должны обновляться без ручной замены бинарника и без превращения канала обновления в цепочку поставки произвольного кода.

Реализовано:

- отдельный runtime updater binary;
- только заранее разрешённые server targets;
- release metadata подписана operational Release Signing Key, разрешённым offline-root trust bundle;
- несколько release sources: Control Plane + независимые зеркала;
- SHA-256 и точный размер артефакта проверяются до установки;
- downgrade запрещён;
- одинаковая версия с другим hash запрещена;
- metadata имеет короткое окно действия;
- новый бинарник ставится атомарно;
- предыдущий binary сохраняется до health-check;
- systemd restart + loopback health-check;
- автоматический rollback при неуспешном старте/health-check;
- отдельный hardened oneshot service + systemd timer;
- независимые timers для нескольких компонентов одного хоста;
- state updater хранится отдельно от runtime component;
- privacy-safe status-файл в /run не содержит секретов;
- Node Agent включает updater status в подписанный heartbeat;
- readiness показывает покрытие runtime updater и ошибки обновлений;
- admin readiness показывает число runtime-нод, updater telemetry и unhealthy updater.

Физическая приёмка:

1. опубликовать staging release server binary версии N+1;
2. проверить автоматическую установку и успешный health-check;
3. опубликовать заведомо нерабочий test binary и подтвердить rollback;
4. проверить отказ downgrade N+1 → N;
5. отключить Control Plane и подтвердить получение release metadata через mirror;
6. выполнить trust-key rotation и подтвердить продолжение обновлений через новый root-authorized release key.


## Спринт 23 — здоровье хостов и самовосстановление нод: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: процесс может быть жив, но хост или VPN worker уже непригоден для клиентского трафика. Такие ноды должны автоматически выводиться из маршрутизации только после устойчивого подтверждения и самостоятельно возвращаться после восстановления.

Реализовано:

- Node Agent публикует uptime, kernel release, размер и свободное место корневой файловой системы;
- локальный worker health остаётся частью подписанного heartbeat;
- критическим локальным состоянием считается worker_healthy=false или менее 5% свободного диска;
- bad/good streak сохраняются в PostgreSQL и переживают перезапуск Control Plane;
- одиночный плохой heartbeat не меняет маршрутизацию;
- после 3 подряд плохих heartbeat active-нода переходит в degraded;
- изменение ноды немедленно вызывает refresh signed Configuration Manifest;
- после 2 подряд здоровых heartbeat нода, деградированная именно local-health автоматикой, возвращается в active;
- manual/circuit-breaker/heartbeat-timeout деградации не снимаются этим механизмом;
- отдельный admin endpoint и экран «Здоровье нод»;
- видны worker health, disk free, uptime, kernel, streak и runtime updater status;
- readiness предупреждает о предаварийных и degraded local-health состояниях;
- системный incident открывается после debounce при auto-degraded нодах;
- отдельный incident создаётся при ошибках signed runtime updater.

Физическая приёмка:

1. заполнить диск тестовой ноды до менее 5% и подтвердить bad streak → degraded;
2. освободить диск и подтвердить два healthy heartbeat → active;
3. остановить локальный VPN worker, оставив Node Agent живым, и проверить исключение ноды;
4. подтвердить, что manual maintenance/quarantine не снимаются автоматикой;
5. проверить refresh manifest и отсутствие новой маршрутизации на degraded ноду.


## Спринт 24 — HA Control Plane и распределённое лидерство: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: несколько экземпляров Control Plane должны одновременно обслуживать API, но конфликтующие singleton-фоновые задачи не должны выполняться параллельно.

Реализовано:

- PostgreSQL advisory lock для singleton leadership;
- лидерство привязано к отдельному DB connection и автоматически снимается PostgreSQL при потере соединения;
- follower проверяет lock каждые 5 секунд и автоматически становится лидером после отказа текущего;
- лидер подтверждает DB-соединение/lease каждые 10 секунд;
- nodemonitor, login cleanup, probe monitor, artifact cleanup, account cleanup, build watchdog и system monitor выполняются только у лидера;
- billing renewal, incident notification и security export остаются distributed work queues на всех репликах;
- config publisher остаётся доступен на каждой HTTP-реплике, чтобы trigger после node mutation не терялся;
- каждый config publish отдельно сериализуется PostgreSQL advisory lock;
- устранён дублирующий trust-bundle startup block в Control Plane;
- состояние singleton leadership сохраняется в PostgreSQL;
- holder id, acquired time, heartbeat и число переходов доступны readiness;
- readiness становится failed при heartbeat лидера старше 30 секунд;
- смена лидера не требует ручного вмешательства.

Физическая приёмка:

1. запустить минимум два Control Plane экземпляра на общей PostgreSQL;
2. подтвердить один singleton leader и два работающих HTTP API;
3. остановить лидера и измерить takeover follower;
4. во время failover выполнить node mutation и подтвердить немедленный signed manifest;
5. разорвать DB connection лидера и подтвердить снятие advisory lock;
6. восстановить экземпляр и убедиться, что он возвращается follower без двойных singleton jobs.


## Спринт 25 — защита Lease и VPN-сессий от злоупотребления: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: валидное устройство не должно иметь возможность бесконтрольно выпускать lease и создавать/пересоздавать worker-сессии, расходуя IPAM, CPU и криптографические операции.

Реализовано:

- persistent rate-limit Access Lease по device ID;
- отдельный persistent rate-limit Proxy Lease;
- лимит считается в минутных bucket и работает одинаково на нескольких Control Plane репликах;
- Access Lease: до 12 выдач в минуту на устройство;
- Proxy Lease: до 30 выдач в минуту, чтобы не ломать browser reconnect/407 recovery;
- превышение возвращает HTTP 429 + Retry-After;
- события учитываются в privacy-preserving security counters;
- старые rate buckets очищаются автоматически;
- worker сохраняет идемпотентность: тот же active device + тот же tunnel public key получает существующую сессию;
- только создание новой/смена ключа учитывается worker burst-limiter;
- worker допускает до 8 новых session starts в минуту на device;
- превышение worker burst-limit возвращает 429, а не маскируется под invalid lease;
- session manager по-прежнему допускает только одну активную VPN-сессию на device на конкретном worker.

Физическая приёмка:

1. выполнить обычные reconnect Android/iOS и убедиться, что лимит не срабатывает;
2. сгенерировать более 12 Access Lease за минуту и подтвердить 429;
3. проверить browser maintenance/407 recovery при Proxy Lease лимите;
4. быстро менять tunnel key более 8 раз в минуту и подтвердить worker 429;
5. проверить очистку rate buckets и восстановление после следующей минуты;
6. прогнать нагрузочный тест на нескольких Control Plane репликах.


## Спринт 26 — независимый off-site DR backup: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: потеря основного Control Plane, локального backup-диска или аккаунта основного провайдера не должна уничтожать единственную пригодную копию состояния.

Реализовано:

- отдельный `vpnx3-backup-replicator`;
- источник — только уже зашифрованный `.tar.age`;
- локальный SHA-256 проверяется до отправки;
- отдельный S3-compatible endpoint/bucket/credentials, независимый от artifact storage;
- timestamped object key считается неизменяемым;
- существующий object с тем же именем и другим hash вызывает fail-closed;
- после upload объект скачивается обратно и повторно проверяется SHA-256;
- off-site success-marker создаётся только после download-back verification;
- отдельный signed Build Factory target `backup_replicator_linux_amd64`;
- hardened systemd oneshot + timer;
- launch readiness отдельно контролирует локальный и off-site backup;
- устранено повторное объявление `trustConfigured` в production config validation.

Эксплуатационные требования:

1. off-site bucket должен находиться в другом account/provider относительно Control Plane;
2. provider-side Object Lock/versioning рекомендуется включить;
3. recovery identity age не хранится рядом с ciphertext backup;
4. реальный restore drill выполняется минимум ежемесячно;
5. потеря локальной копии при наличии off-site должна регулярно имитироваться в staging.

Физическая приёмка:

1. создать локальный encrypted backup;
2. дождаться off-site replication;
3. удалить локальный bundle на staging;
4. скачать только remote ciphertext;
5. расшифровать recovery identity из независимого escrow;
6. выполнить restore drill в пустую PostgreSQL;
7. подтвердить readiness восстановленного Control Plane.


## Спринт 27 — аварийный secret escrow: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: восстановление данных не должно зависеть от тех же production secrets и того же места хранения, что были потеряны вместе с Control Plane.

Реализовано:

- отдельный encrypted DR secret escrow;
- только явный allowlist через повторяемый аргумент `--file label=/absolute/path`;
- source-файлы обязаны быть обычными non-symlink файлами;
- source-файлы обязаны иметь mode 0400/0600;
- manifest содержит label, SHA-256 и размер каждого элемента;
- весь secret bundle шифруется `age`;
- ciphertext получает отдельный SHA-256 sidecar;
- verifier расшифровывает во временный каталог и проверяет каждый item;
- offline Root of Trust private seed явно запрещён политикой этого escrow;
- root seed хранится отдельно/offline и используется для выпуска новых operational keys после DR.

Физическая приёмка:

1. создать escrow с независимым age recipient;
2. хранить ciphertext, age identity и offline root в трёх логически раздельных местах;
3. на чистом DR-host проверить decrypt + manifest;
4. восстановить data backup;
5. выпустить новые config/access/release operational keys;
6. подписать новый trust bundle offline root;
7. поднять изолированный Control Plane и только после readiness публиковать endpoints.


## Спринт 28 — PostgreSQL PITR и непрерывная WAL-репликация: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: уменьшить потенциальную потерю данных с интервала полного backup до минутного уровня.

Реализовано:

- отдельный безопасный PostgreSQL `archive_command`;
- network-independent схема: PostgreSQL сначала пишет только в локальный encrypted spool;
- каждый WAL/archive history file шифруется `age` до попадания во внешнее хранилище;
- атомарное создание ciphertext;
- SHA-256 sidecar каждого encrypted WAL object;
- отдельный `restore_command` для расшифровки WAL в DR;
- отдельный `vpnx3-wal-replicator`;
- независимые S3 credentials/prefix для WAL;
- immutable object semantics: существующий remote object с другим hash вызывает fail-closed;
- remote object читается обратно и проверяется SHA-256;
- bounded local retention после подтверждённой внешней копии;
- backlog_count и latest_local/latest_verified записываются в status marker;
- readiness предупреждает при backlog >16 и становится failed при backlog >128 или отсутствии подтверждения более 30 минут;
- signed Build Factory target `wal_replicator_linux_amd64`;
- hardened systemd timer для частой WAL replication.

Физическая приёмка:

1. включить `wal_level=replica`, `archive_mode=on`, `archive_timeout=300`;
2. выполнить серию транзакций;
3. подтвердить появление encrypted WAL и внешней копии;
4. восстановить base backup на отдельный PostgreSQL;
5. подать WAL через `restore_command`;
6. восстановиться до заданного timestamp/LSN;
7. сравнить ключевые бизнес-таблицы до точки восстановления;
8. измерить реальный RPO/RTO.


## Спринт 29 — автоматизированный DR bootstrap: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: снизить RTO и исключить ручную импровизацию при полном восстановлении Control Plane.

Реализовано:

- `cmd/migrate` больше не зависит от runtime signing/payment secrets;
- migration binary требует только database URL и migrations directory;
- единый `scripts/dr-bootstrap.sh`;
- обязательное подтверждение `DR_ISOLATED_ENVIRONMENT`;
- восстановление в очевидную primary DB по умолчанию запрещено;
- encrypted backup проверяется до restore;
- restore выполняется существующим fail-closed drill;
- после restore применяются актуальные migrations;
- вызывается `verify_audit_chain()`;
- проверяются критические таблицы revocation/HA/lease-abuse последних спринтов;
- выводятся контрольные бизнес-счётчики;
- script намеренно не запускает публичный Control Plane и не меняет endpoints.

Физическая приёмка:

1. поднять пустой PostgreSQL в отдельной DR-сети;
2. восстановить только из off-site backup/secret escrow;
3. применить WAL до выбранной точки;
4. прогнать dr-bootstrap;
5. выпустить новые operational keys/root-signed bundle;
6. поднять Control Plane приватно;
7. добиться launch readiness без failed;
8. только после этого переключать клиентские endpoints.


## Спринт 30 — PostgreSQL HA visibility и standby lag: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: HA нескольких Control Plane не должен скрывать единичную точку отказа PostgreSQL.

Реализовано:

- отдельный optional `VPNX3_DATABASE_REPLICA_URL`;
- отдельный малый health-pool, не используемый бизнес-транзакциями;
- strict режим `VPNX3_DATABASE_HA_REQUIRED=true`;
- в production strict mode отсутствие standby блокирует запуск;
- readiness проверяет, что основной endpoint действительно writer (`pg_is_in_recovery=false`);
- readiness проверяет, что replica endpoint действительно standby;
- лаг измеряется по WAL LSN в байтах, а не по возрасту последней транзакции;
- до 64 МБ lag — ok;
- 64–512 МБ — warning;
- более 512 МБ — failed;
- optional replica health-pool сохраняется после transient startup failure и может восстановиться без рестарта Control Plane;
- production transport validation применяется и к replica URL.

Не автоматизируется внутри приложения:

- promotion PostgreSQL standby;
- STONITH/fencing старого writer;
- provider-specific failover.

Эти операции должны выполнять managed PostgreSQL, Patroni/repmgr или другой специализированный HA-контур.

Физическая приёмка:

1. развернуть writer + physical standby на независимых хостах/зонах;
2. включить strict HA;
3. создать WAL lag и проверить warning/failed пороги;
4. остановить standby и подтвердить failed readiness;
5. выполнить контролируемый switchover средствами DB HA-системы;
6. проверить переподключение Control Plane, advisory leadership и очереди после смены writer.


## Спринт 31 — безопасный rolling deploy Control Plane: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: обновление нескольких Control Plane реплик не должно обрывать запросы, удерживать singleton leadership или оставлять terminating-инстанс в балансировщике.

Реализовано:

- явный drain-state внутри HTTP Server;
- при drain `/health/ready` немедленно возвращает HTTP 503 со статусом `draining`;
- `/health/live` остаётся живым до фактической остановки процесса;
- SIGTERM/SIGINT сначала переводит экземпляр в drain;
- фоновые singleton/distributed workers останавливаются до HTTP shutdown;
- PostgreSQL advisory leadership освобождается до завершения процесса;
- configurable `VPNX3_DRAIN_DELAY` даёт балансировщику время убрать экземпляр из новых запросов;
- configurable `VPNX3_SHUTDOWN_TIMEOUT` ограничивает ожидание уже начатых запросов;
- допустимые диапазоны drain/shutdown проверяются при старте;
- параметры задокументированы в `.env.example`.

Физическая приёмка:

1. поднять минимум две Control Plane реплики за балансировщиком;
2. запустить непрерывный поток клиентских запросов;
3. послать SIGTERM текущему leader;
4. подтвердить немедленный readiness=503 только на terminating replica;
5. подтвердить takeover singleton leadership healthy-репликой;
6. убедиться, что новые запросы не идут на draining instance;
7. проверить завершение уже начатых запросов в пределах shutdown timeout;
8. повторить rolling deploy по одной реплике без клиентских 5xx.


## Спринт 32 — HTTP backpressure и защита Control Plane от перегрузки: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: всплеск клиентских/служебных запросов не должен бесконтрольно накапливать goroutine и добивать PostgreSQL/Control Plane.

Реализовано:

- bounded admission controller для всех бизнес/API запросов;
- configurable `VPNX3_HTTP_MAX_INFLIGHT`;
- при насыщении новые запросы получают быстрый HTTP 503 `server_overloaded` + `Retry-After: 1`;
- health/live и health/ready не занимают admission slots и остаются наблюдаемыми при перегрузке;
- счётчики current/peak/rejected_total;
- admission utilization доступен в launch readiness;
- warning при 80% и 95% текущей загрузки;
- configurable `VPNX3_HTTP_MAX_HEADER_KB`;
- HTTP server ограничивает максимальный размер заголовков;
- параметры валидируются при старте.

Физическая приёмка:

1. прогнать bounded load выше лимита inflight;
2. подтвердить быстрые 503 вместо роста latency/timeouts;
3. убедиться, что health endpoints продолжают отвечать;
4. проверить, что после снятия нагрузки admission slots полностью освобождаются;
5. подобрать лимит относительно pgx pool/CPU/RAM по фактической инфраструктуре;
6. повторить тест на двух Control Plane репликах за балансировщиком.


## Спринт 33 — управление PostgreSQL connection pool: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: несколько Control Plane реплик должны потреблять предсказуемое число соединений БД и деградировать наблюдаемо, а не упираться в PostgreSQL внезапно.

Реализовано:

- явная конфигурация primary pgx pool;
- `VPNX3_DATABASE_MAX_CONNS` и `VPNX3_DATABASE_MIN_CONNS`;
- configurable connection lifetime, lifetime jitter и idle time;
- jitter предотвращает одновременную массовую ротацию соединений;
- старый `database.Open` сохранён как совместимый wrapper с безопасными default;
- отдельный health-pool standby остаётся малым и изолированным от бизнес-трафика;
- launch readiness публикует acquired/idle/total/max;
- публикуются empty/canceled acquire counters;
- warning при 80% занятых соединений;
- failed при 95% занятых соединений;
- параметры валидируются до старта.

Физическая приёмка:

1. определить реальный PostgreSQL max_connections;
2. рассчитать budget на число Control Plane реплик + migrations/ops;
3. прогнать HTTP load до admission saturation;
4. подтвердить, что DB pool не достигает 100% раньше admission controller;
5. проверить connection churn с lifetime jitter;
6. проверить rolling deploy двух реплик без connection storm.


## Спринт 34 — SQL timeout и защита пула от зависших запросов: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: медленный SQL, ожидание блокировки или забытая транзакция не должны удерживать connection pool неопределённо долго.

Реализовано:

- PostgreSQL `statement_timeout` для всех primary pool connections;
- отдельный `lock_timeout`;
- `idle_in_transaction_session_timeout`;
- параметры задаются через pgx RuntimeParams на каждое соединение;
- configurable `VPNX3_DATABASE_STATEMENT_TIMEOUT`;
- configurable `VPNX3_DATABASE_LOCK_TIMEOUT`;
- configurable `VPNX3_DATABASE_IDLE_TX_TIMEOUT`;
- lock timeout обязан быть меньше statement timeout;
- отдельный health pool standby имеет более строгие короткие timeout;
- application_name различает business pool и replica-health pool;
- параметры валидируются до старта.

Физическая приёмка:

1. создать искусственный PostgreSQL lock и подтвердить быстрый lock timeout;
2. выполнить заведомо долгий statement и подтвердить server-side cancel;
3. оставить транзакцию idle и проверить принудительное завершение;
4. убедиться, что connection возвращается в pool после ошибки;
5. прогнать обычные billing/config/admin flows и проверить отсутствие ложных timeout;
6. подобрать production thresholds по slow-query telemetry.


## Спринт 35 — приватная HTTP-наблюдаемость: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: оператор должен видеть HTTP-деградацию без сбора содержимого запросов и истории пользовательских IP.

Реализовано:

- in-memory агрегированные HTTP metrics на каждой Control Plane реплике;
- active/total requests;
- разбиение 2xx/3xx/4xx/5xx;
- суммарный response bytes;
- latency buckets <50/<100/<250/<500/<1000/<3000/>=3000 мс;
- отдельный защищённый admin endpoint `/api/v1/admin/http-metrics`;
- экран «HTTP метрики» в административной панели;
- метрики явно помечены как per-replica/process-lifetime;
- request log больше не сохраняет `remote_addr`, чтобы не создавать лишний постоянный IP-след;
- не собираются request body, query string, URL посещаемых ресурсов или VPN-трафик;
- ResponseWriter wrapper поддерживает `Unwrap` для совместимости с стандартным HTTP control path.

Физическая приёмка:

1. прогнать обычный клиентский трафик и проверить счётчики;
2. вызвать контролируемые 4xx/5xx;
3. прогнать bounded load и проверить latency buckets;
4. сравнить показатели двух Control Plane реплик;
5. убедиться, что логи не содержат клиентский IP/body/query;
6. подключить внешний log/metrics collector только при необходимости агрегирования между репликами.


## Спринт 36 — circuit breaker внешних платежей: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: деградация внешнего платёжного API не должна каскадно занимать HTTP admission slots и ресурсы Control Plane.

Реализовано:

- общий concurrency-safe circuit breaker с closed/open/half-open;
- YooKassa открывает circuit после 5 последовательных transport/429/5xx ошибок;
- open cooldown — 30 секунд;
- после cooldown допускается один half-open probe;
- успешный probe полностью закрывает circuit;
- новые client payment requests при open circuit получают быстрый HTTP 503 + Retry-After;
- network/429/5xx ошибки provider verification классифицируются как временная недоступность;
- YooKassa webhook при временной невозможности повторной проверки получает HTTP 503, а не ложный 400;
- malformed/неподдерживаемый webhook по-прежнему получает 400;
- circuit state/failure count/opened_at видны в launch readiness;
- auto-renew использует тот же защищённый provider adapter и также деградирует fail-fast.

Физическая приёмка:

1. имитировать timeout YooKassa API;
2. подтвердить открытие circuit после порога;
3. проверить быстрые 503 без ожидания 12-секундного timeout;
4. проверить half-open single probe после cooldown;
5. подтвердить закрытие после восстановления;
6. проверить повтор webhook после provider outage;
7. убедиться, что malformed webhook не превращается в 503.


## Спринт 37 — устойчивость S3 artifact storage: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: деградация внешнего object storage не должна подвешивать Build Factory, download и readiness на неопределённое время.

Реализовано:

- отдельный circuit breaker для S3-compatible artifact backend;
- threshold 5 последовательных transport/429/5xx ошибок;
- open cooldown 30 секунд и half-open probe;
- fail-fast при открытом circuit;
- повторное использование HTTP/TLS соединений;
- TLS handshake timeout;
- response-header timeout;
- expect-continue timeout;
- bounded idle connection pool;
- circuit state доступен через optional artifact storage reporter;
- launch readiness показывает closed/open/half-open;
- локальный artifact backend не зависит от circuit interface и продолжает работать как раньше.

Физическая приёмка:

1. отключить S3 endpoint и выполнить upload/download/readiness;
2. подтвердить открытие circuit после порога;
3. измерить быстрый fail после открытия;
4. восстановить endpoint и подтвердить half-open → closed;
5. проверить большую загрузку, чтобы response-header timeout не обрывал нормальный streaming body;
6. проверить повторное использование keep-alive соединений.


## Спринт 38 — глобальная политика размера HTTP body: КОДОВАЯ ОСНОВА ЗАВЕРШЕНА

Цель: забытый endpoint без локального MaxBytesReader не должен позволять бесконтрольно читать большой request body.

Реализовано:

- глобальный body-limit middleware до application handlers;
- configurable `VPNX3_HTTP_DEFAULT_BODY_KB`;
- default JSON/API body limit 1 МБ;
- запрос с известным Content-Length выше лимита немедленно получает HTTP 413;
- chunked/неизвестный body ограничивается через `http.MaxBytesReader`;
- build artifact upload выделен как явное исключение и использует `VPNX3_ARTIFACT_MAX_MB`;
- GET/HEAD/OPTIONS не оборачиваются body limiter;
- существующие более строгие локальные лимиты handlers продолжают действовать;
- предел JSON body валидируется при запуске.

Физическая приёмка:

1. отправить oversized JSON с Content-Length и подтвердить быстрый 413;
2. отправить oversized chunked request и подтвердить прекращение чтения;
3. проверить обычные registration/payment/admin payload;
4. загрузить artifact около configured max;
5. проверить отказ artifact выше max;
6. выполнить нагрузочный тест большими body и наблюдать стабильность RAM.
