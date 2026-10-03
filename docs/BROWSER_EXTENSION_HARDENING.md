# Browser client hardening

- Chrome сохраняет callback-based asyncBlocking для proxy auth.
- Firefox использует Promise-based blocking handler.
- Signed Configuration Manifest теперь имеет rollback protection в extension storage.
- created_at не может быть более чем на 5 минут в будущем.
- expires_at обязателен и проверяется до применения proxy configuration.
- Build Worker preflight для chrome_zip/firefox_zip требует HTTPS client control URL и pinned config public key до enrollment/claim.

При ошибке startup reconnect extension выполняет fail-closed: очищает proxy state вместо оставления браузера на неизвестном/просроченном ingress.
