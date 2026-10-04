#!/usr/bin/env bash
set -euo pipefail

CONTROL_URL=""
ENROLLMENT_TOKEN=""
NODE_NAME="probe"
COUNTRY=""
PROVIDER=""
PUBLIC_IP=""
BINARY_URL=""
BINARY_SHA256=""
CONFIG_PUBLIC_KEY=""
INTERVAL_SECONDS="60"
WG_INTERFACE="wgprobe0"
MAX_WORKERS="5"
DATA_PLANE_ENABLED="true"
INSTALL_DEPS="auto"

usage(){
cat <<'EOF'
Установка VPNX3 Probe Node:

  install-probe-node.sh     --control https://CONTROL-ENDPOINT     --token ONE_TIME_PROBE_TOKEN     --name probe-de-01     --country DE     --provider provider-a     --binary-url https://.../vpnx3-probe-agent     --sha256 EXPECTED_SHA256     --config-public-key BASE64URL_ED25519_PUBLIC_KEY     [--public-ip 203.0.113.10]     [--interval 60]     [--wg-interface wgprobe0]     [--max-workers 5]     [--data-plane true|false]     [--install-deps auto|yes|no]

Probe выполняет:
- HTTPS/TLS checks worker session API;
- HTTPS health checks browser ingress;
- synthetic WireGuard lease/session;
- временный WireGuard interface;
- handshake + ping signed gateway_ipv4;
- signed report в Control Plane.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --control) CONTROL_URL="${2:-}"; shift 2;;
    --token) ENROLLMENT_TOKEN="${2:-}"; shift 2;;
    --name) NODE_NAME="${2:-}"; shift 2;;
    --country) COUNTRY="${2:-}"; shift 2;;
    --provider) PROVIDER="${2:-}"; shift 2;;
    --public-ip) PUBLIC_IP="${2:-}"; shift 2;;
    --binary-url) BINARY_URL="${2:-}"; shift 2;;
    --sha256) BINARY_SHA256="${2:-}"; shift 2;;
    --config-public-key) CONFIG_PUBLIC_KEY="${2:-}"; shift 2;;
    --interval) INTERVAL_SECONDS="${2:-}"; shift 2;;
    --wg-interface) WG_INTERFACE="${2:-}"; shift 2;;
    --max-workers) MAX_WORKERS="${2:-}"; shift 2;;
    --data-plane) DATA_PLANE_ENABLED="${2:-}"; shift 2;;
    --install-deps) INSTALL_DEPS="${2:-}"; shift 2;;
    -h|--help) usage; exit 0;;
    *) echo "Неизвестный аргумент: $1" >&2; usage; exit 2;;
  esac
done

[[ "${EUID}" -eq 0 ]] || { echo "Запустите от root." >&2; exit 1; }

required=(CONTROL_URL ENROLLMENT_TOKEN NODE_NAME COUNTRY PROVIDER BINARY_URL BINARY_SHA256 CONFIG_PUBLIC_KEY)
for name in "${required[@]}"; do
  [[ -n "${!name}" ]] || { echo "Не задан обязательный параметр: $name" >&2; exit 2; }
done
[[ "$CONTROL_URL" == https://* ]] || { echo "Control URL должен использовать HTTPS." >&2; exit 2; }
[[ "$WG_INTERFACE" =~ ^[A-Za-z0-9_.-]{1,15}$ ]] || { echo "Некорректное имя probe interface." >&2; exit 2; }
[[ "$INTERVAL_SECONDS" =~ ^[0-9]+$ ]] && (( INTERVAL_SECONDS >= 15 && INTERVAL_SECONDS <= 3600 )) || {
  echo "interval должен быть 15..3600 секунд" >&2; exit 2;
}
[[ "$MAX_WORKERS" =~ ^[0-9]+$ ]] && (( MAX_WORKERS >= 1 && MAX_WORKERS <= 50 )) || {
  echo "max-workers должен быть 1..50" >&2; exit 2;
}
[[ "$DATA_PLANE_ENABLED" == "true" || "$DATA_PLANE_ENABLED" == "false" ]] || {
  echo "data-plane должен быть true или false" >&2; exit 2;
}
[[ "$INSTALL_DEPS" == "auto" || "$INSTALL_DEPS" == "yes" || "$INSTALL_DEPS" == "no" ]] || {
  echo "install-deps должен быть auto, yes или no" >&2; exit 2;
}

install_dependencies(){
  local missing=0
  for cmd in curl sha256sum systemctl install useradd ip wg ping; do
    command -v "$cmd" >/dev/null 2>&1 || missing=1
  done
  [[ "$missing" -eq 0 && "$INSTALL_DEPS" != "yes" ]] && return 0
  [[ "$INSTALL_DEPS" != "no" ]] || { echo "Не установлены зависимости probe node." >&2; exit 1; }
  command -v apt-get >/dev/null 2>&1 || {
    echo "Автоустановка зависимостей поддерживается только для apt-based систем." >&2
    exit 1
  }
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y --no-install-recommends ca-certificates curl wireguard-tools iproute2 iputils-ping
  rm -rf /var/lib/apt/lists/*
}
install_dependencies

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 -o "$tmp" "$BINARY_URL"
actual="$(sha256sum "$tmp" | awk '{print $1}')"
[[ "${actual,,}" == "${BINARY_SHA256,,}" ]] || {
  echo "Контрольная сумма Probe Agent не совпадает." >&2
  exit 1
}

if ! id vpnx3-probe >/dev/null 2>&1; then
  useradd --system --home /var/lib/vpnx3-probe --shell /usr/sbin/nologin vpnx3-probe
fi
install -d -m 0750 -o vpnx3-probe -g vpnx3-probe /var/lib/vpnx3-probe
install -m 0755 -o root -g root "$tmp" /usr/local/bin/vpnx3-probe-agent

cat >/etc/vpnx3-probe.env <<EOF
VPNX3_CONTROL_URL=$CONTROL_URL
VPNX3_ENROLLMENT_TOKEN=$ENROLLMENT_TOKEN
VPNX3_NODE_NAME=$NODE_NAME
VPNX3_NODE_PROVIDER=$PROVIDER
VPNX3_NODE_COUNTRY=$COUNTRY
VPNX3_NODE_PUBLIC_IP=$PUBLIC_IP
VPNX3_AGENT_IDENTITY_PATH=/var/lib/vpnx3-probe/identity.json
VPNX3_CONFIG_CACHE_PATH=/var/lib/vpnx3-probe/config.json
VPNX3_CONFIG_PUBLIC_KEY=$CONFIG_PUBLIC_KEY
VPNX3_PROBE_INTERVAL_SECONDS=$INTERVAL_SECONDS
VPNX3_PROBE_DATA_PLANE_ENABLED=$DATA_PLANE_ENABLED
VPNX3_PROBE_WG_INTERFACE=$WG_INTERFACE
VPNX3_PROBE_DATA_PLANE_MAX_WORKERS=$MAX_WORKERS
EOF
chmod 0600 /etc/vpnx3-probe.env
chown root:root /etc/vpnx3-probe.env

cat >/etc/systemd/system/vpnx3-probe-agent.service <<'EOF'
[Unit]
Description=VPNX3 Distributed Probe Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=vpnx3-probe
Group=vpnx3-probe
EnvironmentFile=/etc/vpnx3-probe.env
ExecStart=/usr/local/bin/vpnx3-probe-agent
Restart=always
RestartSec=5

NoNewPrivileges=true
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
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
ReadWritePaths=/var/lib/vpnx3-probe

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now vpnx3-probe-agent.service

echo "Probe Node установлен."
echo "Проверить:"
echo "  systemctl status vpnx3-probe-agent --no-pager"
echo "  journalctl -u vpnx3-probe-agent -n 100 --no-pager"
