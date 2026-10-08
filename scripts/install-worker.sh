#!/usr/bin/env bash
set -euo pipefail

BINARY_URL=""
BINARY_SHA256=""
ACCESS_PUBLIC_KEY=""
CONTROL_URL=""
REVOCATION_SOURCES=""
WG_INTERFACE="wg0"
WG_ENDPOINT=""
WG_POOL="10.66.0.0/24"
WG_ADDRESS="10.66.0.1/24"
WG_LISTEN_PORT="51820"
PUBLIC_INTERFACE=""
AUTH_ADDR="127.0.0.1:9090"
INSTALL_DEPS="auto"

usage() {
  cat <<'EOF'
Установка VPNX3 VPN Worker на чистый Debian/Ubuntu:

  install-worker.sh \
    --binary-url https://.../vpn-worker \
    --sha256 EXPECTED_SHA256 \
    --access-public-key BASE64URL_ED25519_PUBLIC_KEY \
    --control-url https://CONTROL-ENDPOINT \
    --wg-endpoint PUBLIC_IP_OR_HOST:51820 \
    [--wg-interface wg0] \
    [--wg-pool 10.66.0.0/24] \
    [--wg-address 10.66.0.1/24] \
    [--wg-listen-port 51820] \
    [--public-interface eth0] \
    [--auth-addr 127.0.0.1:9090] \
    [--install-deps auto|yes|no]

Установщик:
- проверяет SHA-256 бинарника;
- устанавливает wireguard-tools/iproute2/iptables при необходимости;
- создаёт приватный ключ WireGuard с правами 0600;
- создаёт wg0 и gateway-адрес;
- включает IPv4 forwarding;
- настраивает NAT только для VPN-подсети;
- включает wg-quick@<interface> и vpnx3-worker.

Важно: WG_ADDRESS обязан быть первым адресом подсети (network + 1).
IPAM VPNX3 выдаёт клиентам адреса начиная с network + 2.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary-url) BINARY_URL="${2:-}"; shift 2 ;;
    --sha256) BINARY_SHA256="${2:-}"; shift 2 ;;
    --access-public-key) ACCESS_PUBLIC_KEY="${2:-}"; shift 2 ;;
    --control-url) CONTROL_URL="${2:-}"; shift 2 ;;
    --revocation-sources) REVOCATION_SOURCES="${2:-}"; shift 2 ;;
    --wg-interface) WG_INTERFACE="${2:-}"; shift 2 ;;
    --wg-endpoint) WG_ENDPOINT="${2:-}"; shift 2 ;;
    --wg-pool) WG_POOL="${2:-}"; shift 2 ;;
    --wg-address) WG_ADDRESS="${2:-}"; shift 2 ;;
    --wg-listen-port) WG_LISTEN_PORT="${2:-}"; shift 2 ;;
    --public-interface) PUBLIC_INTERFACE="${2:-}"; shift 2 ;;
    --auth-addr) AUTH_ADDR="${2:-}"; shift 2 ;;
    --install-deps) INSTALL_DEPS="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Неизвестный аргумент: $1" >&2; usage; exit 2 ;;
  esac
done

[[ "${EUID}" -eq 0 ]] || { echo "Установщик должен быть запущен от root." >&2; exit 1; }

for value in BINARY_URL BINARY_SHA256 ACCESS_PUBLIC_KEY WG_INTERFACE WG_ENDPOINT WG_POOL WG_ADDRESS WG_LISTEN_PORT; do
  [[ -n "${!value}" ]] || { echo "Не задан обязательный параметр: $value" >&2; exit 2; }
done

if [[ -n "$CONTROL_URL" && "$CONTROL_URL" != https://* ]]; then
  echo "Control URL должен использовать HTTPS." >&2
  exit 2
fi
if [[ -z "$CONTROL_URL" && -z "$REVOCATION_SOURCES" ]]; then
  echo "Нужен --control-url или --revocation-sources: worker не запускается без signed revocation feed." >&2
  exit 2
fi

[[ "$WG_INTERFACE" =~ ^[A-Za-z0-9_.-]{1,15}$ ]] || { echo "Некорректное имя WireGuard-интерфейса." >&2; exit 2; }
[[ "$WG_LISTEN_PORT" =~ ^[0-9]+$ ]] || { echo "Некорректный WireGuard port." >&2; exit 2; }
(( WG_LISTEN_PORT >= 1 && WG_LISTEN_PORT <= 65535 )) || { echo "WireGuard port вне диапазона." >&2; exit 2; }
[[ "$INSTALL_DEPS" == "auto" || "$INSTALL_DEPS" == "yes" || "$INSTALL_DEPS" == "no" ]] || {
  echo "install-deps должен быть auto, yes или no" >&2; exit 2;
}

install_dependencies() {
  local missing=0
  for cmd in curl sha256sum systemctl install useradd wg wg-quick ip iptables python3; do
    command -v "$cmd" >/dev/null 2>&1 || missing=1
  done
  [[ "$missing" -eq 0 && "$INSTALL_DEPS" != "yes" ]] && return 0
  [[ "$INSTALL_DEPS" != "no" ]] || { echo "Не установлены обязательные системные зависимости." >&2; exit 1; }
  command -v apt-get >/dev/null 2>&1 || {
    echo "Автоустановка зависимостей поддержана только для apt-based систем. Установите wireguard-tools iproute2 iptables python3 curl вручную." >&2
    exit 1
  }
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y --no-install-recommends     ca-certificates curl wireguard-tools iproute2 iptables python3-minimal
  rm -rf /var/lib/apt/lists/*
}

install_dependencies

python3 - "$WG_POOL" "$WG_ADDRESS" <<'PY'
import ipaddress, sys
pool=ipaddress.ip_network(sys.argv[1], strict=True)
address=ipaddress.ip_interface(sys.argv[2])
if pool.version != 4 or address.version != 4:
    raise SystemExit("VPNX3 first transport supports IPv4 WireGuard pools only")
if pool.prefixlen < 16 or pool.prefixlen > 29:
    raise SystemExit("WG pool prefix must be between /16 and /29")
if address.network != pool:
    raise SystemExit("WG address and WG pool must describe the same network")
expected=next(pool.hosts())
if address.ip != expected:
    raise SystemExit(f"WG gateway must be first host {expected}, got {address.ip}")
PY

if [[ -z "$PUBLIC_INTERFACE" ]]; then
  PUBLIC_INTERFACE="$(ip -4 route show default | awk 'NR==1 {for(i=1;i<=NF;i++) if($i=="dev"){print $(i+1); exit}}')"
fi
[[ "$PUBLIC_INTERFACE" =~ ^[A-Za-z0-9_.-]{1,15}$ ]] || { echo "Не удалось определить безопасный public interface." >&2; exit 2; }
[[ -e "/sys/class/net/$PUBLIC_INTERFACE" ]] || { echo "Public interface $PUBLIC_INTERFACE не найден." >&2; exit 1; }

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 --output "$tmp" "$BINARY_URL"
actual="$(sha256sum "$tmp" | awk '{print $1}')"
[[ "${actual,,}" == "${BINARY_SHA256,,}" ]] || {
  echo "Контрольная сумма VPN Worker не совпадает." >&2
  echo "Ожидалась: $BINARY_SHA256" >&2
  echo "Получена:   $actual" >&2
  exit 1
}

install -d -m 0700 /etc/wireguard
WG_KEY_FILE="/etc/wireguard/${WG_INTERFACE}.key"
if [[ ! -s "$WG_KEY_FILE" ]]; then
  umask 077
  wg genkey >"$WG_KEY_FILE"
fi
chmod 0600 "$WG_KEY_FILE"
WG_PRIVATE_KEY="$(cat "$WG_KEY_FILE")"

cat >/etc/sysctl.d/99-vpnx3-worker.conf <<'EOF'
net.ipv4.ip_forward=1
EOF
sysctl --system >/dev/null

WG_CONFIG="/etc/wireguard/${WG_INTERFACE}.conf"
cat >"$WG_CONFIG" <<EOF
[Interface]
Address = $WG_ADDRESS
ListenPort = $WG_LISTEN_PORT
PrivateKey = $WG_PRIVATE_KEY

PostUp = iptables -C FORWARD -i %i -j ACCEPT 2>/dev/null || iptables -A FORWARD -i %i -j ACCEPT
PostUp = iptables -C FORWARD -o %i -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || iptables -A FORWARD -o %i -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
PostUp = iptables -t nat -C POSTROUTING -s $WG_POOL -o $PUBLIC_INTERFACE -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -s $WG_POOL -o $PUBLIC_INTERFACE -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -j ACCEPT 2>/dev/null || true
PostDown = iptables -D FORWARD -o %i -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true
PostDown = iptables -t nat -D POSTROUTING -s $WG_POOL -o $PUBLIC_INTERFACE -j MASQUERADE 2>/dev/null || true
EOF
chmod 0600 "$WG_CONFIG"

systemctl daemon-reload
systemctl enable "wg-quick@${WG_INTERFACE}.service" >/dev/null
if systemctl is-active --quiet "wg-quick@${WG_INTERFACE}.service"; then
  systemctl restart "wg-quick@${WG_INTERFACE}.service"
else
  systemctl start "wg-quick@${WG_INTERFACE}.service"
fi

if ! ip link show "$WG_INTERFACE" >/dev/null 2>&1; then
  echo "WireGuard interface $WG_INTERFACE did not start." >&2
  exit 1
fi

if ! id vpnx3-worker >/dev/null 2>&1; then
  useradd --system --home /var/lib/vpnx3-worker --shell /usr/sbin/nologin vpnx3-worker
fi
install -d -m 0750 -o vpnx3-worker -g vpnx3-worker /var/lib/vpnx3-worker
install -m 0755 -o root -g root "$tmp" /usr/local/bin/vpnx3-worker

cat >/etc/vpnx3-worker.env <<EOF
VPNX3_ACCESS_PUBLIC_KEY=$ACCESS_PUBLIC_KEY
VPNX3_CONTROL_URL=$CONTROL_URL
VPNX3_REVOCATION_POLL_INTERVAL=60s
VPNX3_WG_INTERFACE=$WG_INTERFACE
VPNX3_WG_ENDPOINT=$WG_ENDPOINT
VPNX3_WG_POOL=$WG_POOL
VPNX3_WORKER_AUTH_ADDR=$AUTH_ADDR
VPNX3_WORKER_STATE_PATH=/var/lib/vpnx3-worker/sessions.json
VPNX3_REVOCATION_STATE_PATH=/var/lib/vpnx3-worker/revocations.json
VPNX3_REVOCATION_SOURCES=$REVOCATION_SOURCES
EOF
chmod 0600 /etc/vpnx3-worker.env
chown root:root /etc/vpnx3-worker.env

cat >/etc/systemd/system/vpnx3-worker.service <<'EOF'
[Unit]
Description=VPNX3 VPN Worker
After=network-online.target wg-quick@WG_INTERFACE.service
Wants=network-online.target
Requires=wg-quick@WG_INTERFACE.service

[Service]
Type=simple
User=vpnx3-worker
Group=vpnx3-worker
EnvironmentFile=/etc/vpnx3-worker.env
ExecStart=/usr/local/bin/vpnx3-worker
Restart=always
RestartSec=3

NoNewPrivileges=true
CapabilityBoundingSet=CAP_NET_ADMIN
AmbientCapabilities=CAP_NET_ADMIN
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
MemoryDenyWriteExecute=true
ReadWritePaths=/var/lib/vpnx3-worker

[Install]
WantedBy=multi-user.target
EOF

sed -i "s/WG_INTERFACE/$WG_INTERFACE/g" /etc/systemd/system/vpnx3-worker.service

systemctl daemon-reload
systemctl enable --now vpnx3-worker.service

WG_PUBLIC_KEY="$(printf '%s' "$WG_PRIVATE_KEY" | wg pubkey)"

echo "VPN Worker установлен."
echo "WireGuard interface: $WG_INTERFACE"
echo "WireGuard gateway:   $WG_ADDRESS"
echo "WireGuard endpoint:  $WG_ENDPOINT"
echo "WireGuard public key: $WG_PUBLIC_KEY"
echo "Public interface:    $PUBLIC_INTERFACE"
echo
echo "Проверить:"
echo "  systemctl status wg-quick@$WG_INTERFACE --no-pager"
echo "  systemctl status vpnx3-worker --no-pager"
echo "  wg show $WG_INTERFACE"
