# Release lifecycle

Release проходит состояния:

`building -> ready -> published -> withdrawn`

Публикация разрешена только если:

- release.status = ready;
- существует хотя бы один build job;
- все build jobs имеют status=succeeded;
- количество зарегистрированных artifacts равно количеству jobs.

Это дополнительная проверка поверх вычисленного status и защищает от рассинхронизации БД/хранилища.

Администратор с `releases.read` может:

- просматривать jobs;
- просматривать artifact metadata;
- скачивать артефакт из защищённого persistent storage.

Загрузка отдаёт заголовок `X-VPNX3-SHA256`, чтобы оператор мог независимо сверить файл.

Public distribution endpoint намеренно не добавлен на этом этапе: сначала нужен подписанный release manifest и политика обновлений клиента.
