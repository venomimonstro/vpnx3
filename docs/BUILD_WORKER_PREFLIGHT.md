# Build Worker preflight

Build worker проходит локальную проверку **до enrollment**.

Для Android обязательны:

- git;
- gradle;
- ANDROID_SDK_ROOT или ANDROID_HOME;
- существующий SDK directory;
- release keystore path/password/alias/key password;
- VPNX3_CLIENT_CONTROL_URL;
- VPNX3_CONFIG_PUBLIC_KEY.

Для browser targets нужен zip. Для iOS — macOS + xcodebuild.

После enrollment build-worker отправляет обычный подписанный heartbeat каждые 30 секунд с build targets и preflight status.

Очередь build jobs выдаёт задания только build_worker со status=active. Поэтому новый worker сначала должен пройти штатный lifecycle testing -> draft -> active через административную панель. Это исключает автоматическое выполнение релизных сборок на только что появившейся машине.
