# Runtime Updater

Отдельный root-owned oneshot проверяет обновления серверных компонентов по подписанному release channel.

Инварианты:

- доверие начинается с Offline Root Public Key;
- release key берётся только из root-signed trust bundle;
- target локально сопоставлен с бинарником и systemd unit;
- удалённый release не задаёт shell-команду, путь установки или имя сервиса;
- downgrade запрещён локальной baseline-версией;
- artifact проверяется по точному размеру и SHA-256;
- замена бинарника атомарная;
- предыдущий бинарник хранится до успешного health-check;
- при неуспехе выполняется rollback;
- updater запускается systemd timer как oneshot, а не постоянно привилегированным демоном.

Rollout: сначала canary-нода, затем небольшая группа, затем остальной парк после проверки probes/health.
