# Build Worker

`cmd/build-worker` — отдельный агент сборки.

## Инварианты безопасности

- Source repository задаётся локальной конфигурацией worker, а не build job.
- Job содержит только allowlisted target и точный 40-символьный commit.
- После checkout worker повторно выполняет `git rev-parse HEAD` и сравнивает commit.
- Команды запускаются через `exec.CommandContext`, без `sh -c`.
- Build timeout ограничен.
- Рабочий каталог временный и удаляется после задания.
- Android signing secrets читаются только из окружения build-worker.
- Control Plane никогда не получает keystore password/private signing key.

## Android signing

На build-worker:

```
VPNX3_ANDROID_KEYSTORE_PATH=/secure/release.jks
VPNX3_ANDROID_KEYSTORE_PASSWORD=...
VPNX3_ANDROID_KEY_ALIAS=...
VPNX3_ANDROID_KEY_PASSWORD=...
```

Gradle подключает release signing только если присутствуют все четыре секрета.

## Текущий предел

Worker умеет физически собрать APK/AAB при наличии Android toolchain, но намеренно **не помечает job succeeded**, пока артефакт не сохранён в durable artifact storage. Следующий подспринт — защищённая загрузка артефакта и только затем финализация job.
