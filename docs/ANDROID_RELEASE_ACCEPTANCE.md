# Android release acceptance

После Build Factory, до загрузки APK/AAB в магазин:

```bash
bash scripts/validate-android-release.sh --apk app-release.apk --aab app-release.aab
```

Проверяется:

- криптографическая подпись APK;
- JAR-подпись AAB;
- `INTERNET`;
- `allowBackup=false`;
- `usesCleartextTraffic=false`;
- отсутствие случайно добавленных чувствительных разрешений: контакты, SMS, журнал звонков, геолокация, микрофон, камера.

После этого всё равно обязательны:

- установка на реальное Android-устройство;
- первый запуск;
- разрешение VPN;
- подключение/отключение;
- смена сети Wi‑Fi ↔ мобильная;
- Always-on/lockdown;
- оплата и возврат из браузера;
- личный ключ на hardware-backed/StrongBox устройстве.
