# Artifact upload

Build artifacts загружаются потоково и не буферизуются целиком в RAM.

## Аутентификация

Worker перед PUT вычисляет SHA-256 файла и подписывает:

- HTTP method;
- endpoint path;
- timestamp;
- SHA-256 как canonical body.

В заголовках отправляются:

- X-VPNX3-Node-ID;
- X-VPNX3-Timestamp;
- X-VPNX3-Signature;
- X-VPNX3-Sequence;
- X-VPNX3-Content-SHA256.

После проверки identity/replay сервер принимает поток, независимо вычисляет SHA-256 и сравнивает его с подписанным значением.

## Атомарность

1. upload пишется во временный файл;
2. hash проверяется;
3. файл атомарно переименовывается;
4. в одной DB-транзакции создаётся release_artifact;
5. job меняется running -> succeeded;
6. release становится ready только если все его jobs succeeded.

При ошибке регистрации окончательный файл удаляется.

По умолчанию артефакты хранятся в отдельном persistent volume Control Plane. Для масштабирования следующий storage adapter сможет заменить локальный диск на S3-compatible backend без изменения build protocol.
