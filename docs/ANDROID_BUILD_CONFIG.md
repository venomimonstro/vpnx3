# Android build configuration

У Build Worker теперь разделены два адреса:

- `VPNX3_CONTROL_URL` — внутренний/операторский адрес Control Plane, по которому сам build-worker получает jobs;
- `VPNX3_CLIENT_CONTROL_URL` — публичный HTTPS endpoint, который будет зашит в APK и доступен устройствам пользователей.

Это принципиально разные значения.

Release Android build выполняет fail-fast, если:

- client control URL не HTTPS;
- client URL остался 127.0.0.1;
- отсутствует pinned Configuration public key.

Gradle читает параметры как из Gradle properties, так и из environment:

```
VPNX3_CLIENT_CONTROL_URL=https://...
VPNX3_CONFIG_PUBLIC_KEY=<base64url>
VPNX3_RELEASE_PUBLIC_KEY=<base64url>
```

Release public key может быть пустым, если update channel сознательно отключён. Configuration public key для release build обязателен.

Build Worker installer поддерживает соответствующие параметры и сохраняет их в root-owned environment file.
