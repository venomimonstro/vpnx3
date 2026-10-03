#!/usr/bin/env bash
set -euo pipefail

BINARY_URL=""
BINARY_SHA256=""
ACCESS_PUBLIC_KEY=""
WG_INTERFACE="wg0"
WG_ENDPOINT=""
WG_POOL="10.66.0.0/24"
AUTH_ADDR="127.0.0.1:9090"

usage() {
  cat <<'EOF'
Установка VPNX3 VPN Worker:

  install-worker.sh \
    --binary-url https://.../vpn-worker \
    --sha256 EXPECTED_SHA256 \
    --access-public-key BASE64URL_ED25519_PUBLIC_KEY \
    --wg-interface wg0 \
    --wg-endpoint SERVER_IP:51820 \
    [--wg-pool 10.66.0.0/24]

WireGuard-интерфейс должен быть создан и настроен заранее.
Установщик проверяет SHA-256 бинарника перед установкой.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary-url) BINARY_URL="${2:-}"; shift 2 ;;
    --sha256) BINARY_SHA256="${2:-}"; shift 2 ;;
    --access-public-key) ACCESS_PUBLIC_KEY="${2:-}"; shift 2 ;;
    --wg-interface) WG_INTERFACE="${2:-}"; shift 2 ;;
    --wg-endpoint) WG_ENDPOINT="${2:-}"; shift 2 ;;
    --wg-pool) WG_POOL="${2:-}"; shift 2 ;;
    --auth-addr) AUTH_ADDR="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Неизвестный аргумент: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ "${EUID}" -ne 0 ]]; then
  echo "Установщик должен быть запущен от root." >&2
  exit 1
fi

for value in BINARY_URL BINARY_SHA256 ACCESS_PUBLIC_KEY WG_INTERFACE WG_ENDPOINT; do
  if [[ -z "${!value}" ]]; then
    echo "Не задан обязательный параметр: $value" >&2
    exit 2
  fi
done

for cmd in curl sha256sum systemctl install useradd; do
  command -v "$cmd" >/dev/null 2>&1 || {
    echo "Не найдена обязательная команда: $cmd" >&2
    exit 1
  }
done

if [[ ! -e "/sys/class/net/$WG_INTERFACE" ]]; then
  echo "Интерфейс WireGuard $WG_INTERFACE не найден." >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

curl --fail --silent --show-error --location --proto '=https' --tlsv1.2   --output "$tmp" "$BINARY_URL"

actual="$(sha256sum "$tmp" | awk '{print $1}')"
if [[ "${actual,,}" != "${BINARY_SHA256,,}" ]]; then
  echo "Контрольная сумма VPN Worker не совпадает." >&2
  exit 1
fi

if ! id vpnx3-worker >/dev/null 2>&1; then
  useradd --system --home /var/lib/vpnx3-worker --shell /usr/sbin/nologin vpnx3-worker
fi

install -d -m 0750 -o vpnx3-worker -g vpnx3-worker /var/lib/vpnx3-worker
install -m 0755 -o root -g root "$tmp" /usr/local/bin/vpnx3-worker

cat >/etc/vpnx3-worker.env <<EOF
VPNX3_ACCESS_PUBLIC_KEY=$ACCESS_PUBLIC_KEY
VPNX3_WG_INTERFACE=$WG_INTERFACE
VPNX3_WG_ENDPOINT=$WG_ENDPOINT
VPNX3_WG_POOL=$WG_POOL
VPNX3_WORKER_AUTH_ADDR=$AUTH_ADDR
EOF
chmod 0600 /etc/vpnx3-worker.env
chown root:root /etc/vpnx3-worker.env

cat >/etc/systemd/system/vpnx3-worker.service <<'EOF'
[Unit]
Description=VPNX3 VPN Worker
After=network-online.target
Wants=network-online.target

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

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now vpnx3-worker.service

echo "VPN Worker установлен."
echo "Проверить: systemctl status vpnx3-worker --no-pager"
