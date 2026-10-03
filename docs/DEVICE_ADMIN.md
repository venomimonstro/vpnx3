# Device administration

Администратор с `users.manage` может отозвать или повторно активировать устройство.

## Revoke

`POST /api/v1/admin/users/{userId}/devices/{deviceId}/revoke`

Меняет:

- status: active -> revoked;
- revoked_at: now.

После этого Control Plane не выдаёт устройству новые Access Lease, потому что `DeviceAuthState` больше не active.

## Offline limitation

Worker проверяет уже выданный Access Lease локально, без постоянного запроса к Control Plane. Поэтому отзыв устройства не может мгновенно уничтожить уже существующую offline-сессию без отдельного revocation channel.

Текущая гарантия: активная сессия завершится не позже `expires_at` Access Lease, затем worker sweeper удалит peer.

Если продукту потребуется мгновенный revoke, следующий уровень — подписанный revocation epoch/list, распространяемый на workers. Это сознательный trade-off между мгновенным контролем и устойчивостью при недоступном Control Plane.
