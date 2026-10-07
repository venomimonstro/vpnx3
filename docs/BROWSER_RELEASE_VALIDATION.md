# Browser extension release validation

Перед упаковкой Chrome/Firefox Build Factory должен проверять исходный каталог расширения.

Проверяются:

- Manifest V3;
- корректная версия;
- только ожидаемые permissions;
- обязательные proxy-auth permissions для конкретного браузера;
- production HTTPS Control URL;
- pinned Configuration Signing Key;
- отсутствие localhost/127.0.0.1;
- отсутствие приватных ключей, паролей, токенов и secret-переменных;
- наличие background/popup файлов.

Ручной запуск:

```bash
python3 scripts/validate-browser-extension.py --browser chrome --dir apps/browser-extension/chrome
python3 scripts/validate-browser-extension.py --browser firefox --dir apps/browser-extension/firefox
```

Build Factory запускает ту же проверку перед ZIP.
