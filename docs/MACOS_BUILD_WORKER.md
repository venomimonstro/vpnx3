# macOS Build Worker

Для iOS Build Factory используется отдельный macOS/Apple Silicon worker.

## Что уже поддерживается

- Build Factory выпускает `build_worker_darwin_arm64`;
- бинарник распространяется через общий signed release channel;
- `scripts/install-macos-build-worker.sh` проверяет SHA-256;
- worker запускается через пользовательский `launchd`;
- enrollment и build protocol остаются теми же подписанными Ed25519-протоколами;
- Xcode signing identities и provisioning profiles не передаются Control Plane и остаются в локальном macOS Keychain.

## Почему iOS IPA пока не включён

В репозитории ещё отсутствует `apps/ios` и Xcode project/workspace. Текущий `ios_ipa` target намеренно завершается ошибкой вместо создания фиктивного артефакта.

Следующий шаг после появления iOS приложения:

1. определить scheme/workspace;
2. добавить фиксированный `xcodebuild archive`;
3. экспортировать IPA по заранее заданному ExportOptions.plist;
4. проверить bundle version/version name из Release;
5. подписать штатными Apple signing assets на macOS worker;
6. загрузить IPA в существующий artifact storage.
