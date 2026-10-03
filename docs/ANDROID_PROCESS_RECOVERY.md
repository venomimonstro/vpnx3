# Android process recovery

VPNX3 не создаёт второй самописный VPN Service поверх WireGuard GoBackend.

Вместо этого исправлен реальный источник рассинхронизации после process death:

- состояние туннеля читается через `GoBackend.getState()`, а не только из volatile-поля процесса;
- после успешного connect локально сохраняется только metadata worker session:
  session ID и HTTPS session endpoint;
- WireGuard private key и конфигурация туннеля в SharedPreferences не сохраняются;
- после рестарта приложения backend state определяет CONNECTED/DISCONNECTED;
- если backend уже DOWN, но metadata worker session осталась, приложение закрывает stale server-side session и очищает metadata;
- disconnect после process restart способен закрыть worker peer по сохранённому session ID.

Это даёт корректное восстановление состояния без дублирования Android VPN lifecycle.
