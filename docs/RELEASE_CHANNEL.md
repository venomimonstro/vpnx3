# Signed release channel

Публичный release channel включается только при наличии отдельного `VPNX3_RELEASE_SIGNING_KEY`.

Он не использует Configuration Signing Key и Access Signing Key.

`GET /api/v1/releases/latest?target=android_apk` возвращает envelope:

- exact payload bytes в Base64URL;
- Ed25519 signature;
- key_id.

Payload содержит только опубликованный артефакт:

- release/version;
- target;
- artifact ID;
- file name;
- SHA-256;
- size;
- download path.

Android закрепляет `VPNX3_RELEASE_PUBLIC_KEY` во время сборки и проверяет подпись локально до чтения metadata.

Public download разрешён только для release со status=published.

На этом этапе Android лишь показывает наличие версии. Автоматическая установка APK не реализована: канал обновлений отделён от механизма установки, чтобы не смешивать криптографическую проверку релиза с правилами Google Play/direct distribution.
