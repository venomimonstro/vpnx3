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
