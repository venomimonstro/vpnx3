# Distributed probes

Probe — независимая нода наблюдения с ролью `probe` и собственной Ed25519 identity.

## Контуры проверки

Каждый цикл probe:

1. получает подписанный Configuration Manifest и проверяет Ed25519-подпись;
2. проверяет HTTPS/TLS доступность `session_api` worker-нод;
3. проверяет HTTPS health browser ingress;
4. выполняет synthetic WireGuard data-plane probe для ограниченного набора worker-нод;
5. отправляет подписанный отчёт в Control Plane;
6. использует отдельные монотонные sequence для reports и synthetic Access Lease.

## Synthetic WireGuard probe

Это не UDP port-check. Probe проходит клиентский путь до Data Plane:

1. генерирует одноразовую WireGuard key pair;
2. подписанным запросом probe-node получает probe-only Access Lease на 2 минуты;
3. обращается к HTTPS session API конкретного worker;
4. worker создаёт временный peer через обычный session manager/IPAM;
5. probe поднимает временный интерфейс `wgprobe0`;
6. конфигурирует server public key и endpoint только из signed manifest/session response;
7. назначает выданный worker адрес;
8. создаёт route только к подписанному `network.gateway_ipv4`;
9. выполняет ping внутреннего gateway через WireGuard;
10. проверяет свежий `LastHandshakeTime` через wgctrl;
11. удаляет worker-session и локальный временный интерфейс.

Успех `wireguard_data_plane` означает, что реально сработали: Control Plane lease, worker authorization, IPAM, создание WireGuard peer, UDP handshake и передача пакета через tunnel.

## Ограничение нагрузки

Probe не обязан проверять все worker каждую минуту. `VPNX3_PROBE_DATA_PLANE_MAX_WORKERS` ограничивает число synthetic checks за цикл; выбор worker детерминированно ротируется по probe node id и минутному bucket.

## Health и circuit breaker

Control Plane хранит observations и пересчитывает health score по свежему окну. HTTP/ingress и `wireguard_data_plane` являются независимыми observations. Изменения circuit breaker инициируют новую подписанную конфигурацию, поэтому клиенты получают обновлённый routing pool автоматически.

## Эксплуатационные требования probe-host

Для data-plane проверки нужны:

- Linux;
- `wireguard-tools`, `iproute2`, `iputils-ping`;
- `CAP_NET_ADMIN` для временного WireGuard interface;
- `CAP_NET_RAW` для ICMP;
- HTTPS-доступ к Control Plane и worker session API;
- UDP-доступ к WireGuard endpoints.

`scripts/install-probe-node.sh` устанавливает зависимости на apt-based системах и создаёт hardened systemd unit.
