# Access Lease binding

Access Lease является короткоживущим подписанным правом доступа, но начиная с этого спринта он не является свободно переносимым bearer-токеном.

При запросе lease устройство отправляет `tunnel_public_key` внутри подписанного device-запроса. Control Plane помещает этот WireGuard public key в подписанные claims Access Lease.

VPN Worker при создании сессии проверяет одновременно:

- подпись Access Lease;
- срок действия;
- идентификатор устройства;
- право trial/подписки;
- точное совпадение `tunnel_public_key` из lease с public key, который клиент просит установить как WireGuard peer.

Поэтому перехваченный Access Lease нельзя использовать с другой WireGuard key pair.
