# Подписанная сетевая политика клиента

Configuration Manifest теперь содержит блок `network`:

```json
{
  "dns_servers": [],
  "mtu": 1280,
  "persistent_keepalive_seconds": 25
}
```

Эти параметры входят в подписанный payload, поэтому Android не принимает их из неподписанного worker-ответа.

Android также больше не ограничен первым worker. Он сортирует активные worker по endpoint priority и health score и последовательно пытается создать сессию. Один Access Lease может безопасно использоваться для failover, потому что он привязан к той же WireGuard public key.

Если пользователь отклоняет системное VPN-разрешение Android, подготовленная worker session немедленно удаляется.
