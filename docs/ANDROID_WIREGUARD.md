# Android WireGuard flow

VPNX3 Android использует официальную embeddable WireGuard tunnel library.

Путь подключения:

1. Android проверяет подписанный Configuration Manifest.
2. Из списка `workers` выбирается нода, имеющая одновременно HTTPS `session_api` и UDP `wireguard` endpoint.
3. Получается короткоживущий Access Lease.
4. Отдельная Curve25519 WireGuard key pair загружается из защищённого локального хранилища или создаётся впервые.
5. Private key шифруется AES-GCM ключом из Android Keystore. Identity key устройства и WireGuard key — разные ключи.
6. Клиент отправляет Access Lease и WireGuard public key в HTTPS session API worker.
7. Worker возвращает assigned IP, WireGuard server public key и endpoint.
8. Endpoint должен совпасть с endpoint из подписанного Configuration Manifest.
9. Создаётся WireGuard Config и поднимается через официальный GoBackend.
10. При отключении Android опускает туннель и удаляет сессию на worker.

Важно: `session_api` должен быть опубликован наружу через HTTPS с валидным TLS-сертификатом. Внутренний worker по умолчанию слушает loopback, поэтому production deployment должен ставить перед ним HTTPS reverse proxy или ingress.
