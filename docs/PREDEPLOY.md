# Ручной predeploy gate

Проект намеренно не использует GitHub Actions/CI. Перед production update запускается:

```bash
bash scripts/predeploy-check.sh
```

Быстрый вариант для локальной правки:

```bash
bash scripts/predeploy-check.sh --fast
```

Полная проверка включает:

1. отсутствие GitHub workflow-файлов;
2. `git diff --check`;
3. уникальность номеров SQL migrations;
4. `bash -n` всех shell-скриптов;
5. Python syntax compile;
6. JavaScript syntax через `node --check`, если Node установлен;
7. обязательный `gofmt`;
8. `go vet ./...`;
9. `go test ./...`.

Скрипт **не выполняет deploy**, не меняет сервер, не применяет миграции и не требует CI.

После успешного predeploy gate для production всё равно выполняются:

- production smoke;
- readiness;
- при сетевых изменениях — controlled failover drill;
- после значимых инфраструктурных изменений — bounded load/soak.
