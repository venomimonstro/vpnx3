#!/usr/bin/env bash
set -euo pipefail

CONTROL_URL=""
ENROLLMENT_TOKEN=""
NODE_NAME=""
COUNTRY=""
PROVIDER=""
BINARY_URL=""
BINARY_SHA256=""
CAPACITY="1000"

usage() {
  cat <<'EOF'
Использование:
  install-node.sh \
    --control https://CONTROL-ENDPOINT \
    --token ONE_TIME_TOKEN \
    --name de-01 \
    --country DE \
    --provider provider-a \
    --binary-url https://.../node-agent \
    --sha256 EXPECTED_SHA256 \
    [--capacity 1000]

SHA-256 бинарника обязателен.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --control) CONTROL_URL="${2:-}"; shift 2 ;;
    --token) ENROLLMENT_TOKEN="${2:-}"; shift 2 ;;
    --name) NODE_NAME="${2:-}"; shift 2 ;;
    --country) COUNTRY="${2:-}"; shift 2 ;;
    --provider) PROVIDER="${2:-}"; shift 2 ;;
    --binary-url) BINARY_URL="${2:-}"; shift 2 ;;
    --sha256) BINARY_SHA256="${2:-}"; shift 2 ;;
    --capacity) CAPACITY="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Неизвестный аргумент: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ "${EUID}" -ne 0 ]]; then
  echo "Установщик должен быть запущен от root." >&2
  exit 1
fi

for value in CONTROL_URL ENROLLMENT_TOKEN NODE_NAME COUNTRY PROVIDER BINARY_URL BINARY_SHA256; do
  if [[ -z "${!value}" ]]; then
    echo "Не задан обязательный параметр: ${value}" >&2
    exit 2
  fi
done

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "Поддерживается только Linux." >&2
  exit 1
fi

for cmd in curl sha256sum systemctl install useradd; do
  command -v "$cmd" >/dev/null 2>&1 || {
    echo "Не найдена обязательная команда: $cmd" >&2
    exit 1
  }
done

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

echo "Скачивание Node Agent..."
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
  --output "$tmp" "$BINARY_URL"

actual="$(sha256sum "$tmp" | awk '{print $1}')"
if [[ "${actual,,}" != "${BINARY_SHA256,,}" ]]; then
  echo "Контрольная сумма Node Agent не совпадает." >&2
  echo "Ожидалась: $BINARY_SHA256" >&2
  echo "Получена:   $actual" >&2
  exit 1
fi

if ! id vpnx3 >/dev/null 2>&1; then
  useradd --system --home /var/lib/vpnx3-agent --shell /usr/sbin/nologin vpnx3
fi

install -d -m 0700 -o vpnx3 -g vpnx3 /var/lib/vpnx3-agent
install -m 0755 -o root -g root "$tmp" /usr/local/bin/vpnx3-node-agent

cat >/etc/vpnx3-agent.env <<EOF
VPNX3_CONTROL_URL=$CONTROL_URL
VPNX3_ENROLLMENT_TOKEN=$ENROLLMENT_TOKEN
VPNX3_NODE_NAME=$NODE_NAME
VPNX3_NODE_COUNTRY=$COUNTRY
VPNX3_NODE_PROVIDER=$PROVIDER
VPNX3_NODE_CAPACITY=$CAPACITY
VPNX3_AGENT_IDENTITY_PATH=/var/lib/vpnx3-agent/identity.json
VPNX3_WORKER_STATUS_URL=http://127.0.0.1:9090/internal/v1/status
EOF
chmod 0600 /etc/vpnx3-agent.env
chown root:root /etc/vpnx3-agent.env

cat >/etc/systemd/system/vpnx3-node-agent.service <<'EOF'
[Unit]
Description=VPNX3 Node Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=vpnx3
Group=vpnx3
EnvironmentFile=/etc/vpnx3-agent.env
ExecStart=/usr/local/bin/vpnx3-node-agent
Restart=always
RestartSec=5
NoNewPrivileges=true
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
ReadWritePaths=/var/lib/vpnx3-agent

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now vpnx3-node-agent.service

echo "Node Agent установлен."
echo "Проверить состояние: systemctl status vpnx3-node-agent --no-pager"
