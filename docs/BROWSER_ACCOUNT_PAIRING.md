# Browser multi-device account

Chrome и Firefox теперь используют тот же account pairing, что Android.

В popup:
- отображается текущий entitlement;
- показано active_devices / device_limit;
- оплаченный аккаунт может создать одноразовый pairing code;
- новый browser-extension device может ввести код с Android или другого устройства;
- после успешного claim local user_id обновляется, а следующий Proxy Lease выдаётся уже по общей подписке.

Proxy credential не переносится между аккаунтами вручную: после pairing серверная entitlement-проверка остаётся источником истины.
