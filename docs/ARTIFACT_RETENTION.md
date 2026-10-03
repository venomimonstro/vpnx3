# Artifact retention

Control Plane запускает отдельный retention cleaner.

Автоматически удаляются только:

- артефакты release со status=withdrawn;
- release должен оставаться withdrawn дольше VPNX3_ARTIFACT_RETENTION_DAYS;
- временные upload-* файлы старше 24 часов.

Автоматика **не удаляет** артефакты published, ready или building-релизов.

Удаление идёт в порядке:

1. проверка, что storage_key остаётся внутри ArtifactDir;
2. удаление файла;
3. удаление metadata release_artifact из PostgreSQL.

Если файловая операция не удалась, строка БД остаётся и будет обработана следующим проходом. Это предпочтительнее orphaning metadata без понимания, сохранился ли бинарник.

Значение retention по умолчанию: 30 дней, допустимый диапазон 1–3650.
