# Manual production release check

Проект сознательно не использует GitHub Actions/CI. Перед production publish можно выполнить единый read-only preflight:

```bash
export VPNX3_CHECK_CONTROL_URL=https://control.example
export VPNX3_CHECK_ADMIN_EMAIL=owner@example.com
export VPNX3_CHECK_ADMIN_PASSWORD='...'
export VPNX3_CHECK_RELEASE_ID='<release uuid>'

bash scripts/production-release-check.sh
```

Exit codes:

- `0` — проверки прошли;
- `10` — launch readiness содержит `failed`;
- `11` — production release gate содержит blockers.

При фактическом publish сервер дополнительно повторяет gate и сохраняет append-only `release_publication_attestations`: кто опубликовал релиз, окружение, gate status, blockers/warnings и ключевые сигналы на момент проверки.
