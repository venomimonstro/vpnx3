# Установка browser ingress

Установщик: `scripts/install-ingress-node.sh`.

Он не скачивает «latest» без проверки: для node-agent и ingress-proxy обязательны URL и ожидаемый SHA-256.

Пример параметров:

```
sudo ./scripts/install-ingress-node.sh \
  --node-agent-url https://.../vpnx3-node-agent \
  --node-agent-sha256 <sha256> \
  --ingress-url https://.../vpnx3-ingress-proxy \
  --ingress-sha256 <sha256> \
  --control-url https://control.example \
  --enrollment-token <one-time ingress token> \
  --access-public-key <base64url Ed25519 public key> \
  --tls-cert /etc/letsencrypt/live/.../fullchain.pem \
  --tls-key /etc/letsencrypt/live/.../privkey.pem \
  --node-name ingress-eu-1 \
  --country NL \
  --provider provider-name \
  --public-ip 203.0.113.10 \
  --listen :8443
```

После установки нода не становится production автоматически.

Оператор обязан:

1. дождаться authenticated heartbeat и статуса testing;
2. выполнить testing -> draft;
3. добавить endpoint:
   - kind=ingress
   - transport=http-connect
   - scheme=https
   - host=публичный DNS/IP
   - port=TLS port
4. выполнить draft -> active;
5. опубликовать новый Configuration Manifest.

Это сохраняет ручной production gate и не позволяет скомпрометированной новой машине самой объявить себя публичной точкой сети.
