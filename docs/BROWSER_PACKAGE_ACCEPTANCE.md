# Browser package acceptance

После Build Factory:

```bash
python3 scripts/validate-browser-package.py --browser chrome --zip vpnx3-chrome.zip
python3 scripts/validate-browser-package.py --browser firefox --zip vpnx3-firefox.zip
```

Проверяются безопасное содержимое ZIP и те же production-инварианты, что для исходного каталога расширения.

Физическая приёмка всё ещё обязательна в актуальных Chrome/Firefox:

1. чистый профиль браузера;
2. установка расширения;
3. первый вход/регистрация;
4. Connect/Disconnect;
5. proxy authentication;
6. обновление Proxy Lease;
7. переключение ingress после failover;
8. отсутствие перехода в DIRECT при краткой недоступности Control Plane.
