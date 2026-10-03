# Hardening notes

- Login throttle использует типизированную проверку `errors.Is(err, pgx.ErrNoRows)`, а не анализ текста ошибки драйвера.
- Signed Release Manifest теперь имеет `issued_at` и `expires_at` (10 минут). Android отклоняет просроченные и явно будущие метаданные.
- Android сравнивает verified release version с `BuildConfig.VERSION_NAME` и не показывает установленную версию как доступное обновление.
