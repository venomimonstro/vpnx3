# Browser HTTPS ingress

Браузерный транспорт отделён от WireGuard.

Расширение регистрируется как device (platform chrome/firefox), затем подписанным device request получает короткоживущий Proxy Lease через POST /api/v1/client/proxy-lease.

Proxy Lease содержит scope browser_proxy, поэтому не взаимозаменяем с WireGuard Access Lease.

cmd/ingress-proxy — HTTPS forward proxy. Username для proxy auth: vpnx3, password: Base64URL signed Proxy Lease envelope.

Ingress проверяет Ed25519 подпись и expiry офлайн.

SSRF-защита:
- destination ports только 80/443;
- запрещены loopback/private/link-local/multicast/unspecified;
- блокируется CGNAT 100.64/10;
- DNS разрешается на ingress, dial выполняется на уже проверенный IP;
- Proxy-Authorization не пересылается целевому сайту.

HTTPS сайты идут через CONNECT tunnel, HTTP — обычным forward proxy.
