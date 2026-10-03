# Artifact download integrity

Административное скачивание выполняется authenticated fetch с Bearer-токеном. Прямой переход браузера по защищённому URL не используется, потому что navigation request не содержит Authorization header из sessionStorage.

Перед отдачей административного артефакта Control Plane повторно вычисляет SHA-256 файла и сравнивает его с immutable metadata в БД.

Если persistent volume повреждён или файл заменён:

- download возвращает 409 artifact_integrity_failed;
- ошибка записывается в server log;
- повреждённый бинарник не отдаётся оператору.

Это дополнение к SHA-256, который проверяется при первоначальной загрузке build-worker.
