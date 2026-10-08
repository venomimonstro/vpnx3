# Local quality gate

CI/GitHub Actions intentionally are not used. Перед production rollout следует вручную запускать:

```bash
bash scripts/local-quality-gate.sh
bash scripts/migration-smoke.sh
```

`local-quality-gate.sh` проверяет:

- уникальность migration versions;
- gofmt;
- unit tests критичных пакетов;
- iOS source security invariants;
- Chrome/Firefox manifests;
- очевидные private-key/credential patterns.

`migration-smoke.sh` поднимает временный PostgreSQL 16 container, применяет **все** migrations через реальный `cmd/migrate` и проверяет:

- число применённых migrations совпадает с файлами;
- audit hash chain;
- append-only audit trigger;
- historical 000019 repair;
- recurring/release schema.

Исторический duplicate migration `000019` исправлен новой идемпотентной repair migration `000027`. Migrator теперь fail-closed при любой будущей duplicate version.
