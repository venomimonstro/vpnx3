#!/usr/bin/env bash
set -euo pipefail

CONTROL_URL=""
ENROLLMENT_TOKEN=""
NODE_NAME=""
COUNTRY=""
PROVIDER=""
PUBLIC_IP=""
CAPACITY="1000"

NODE_AGENT_URL=""
NODE_AGENT_SHA256=""
WORKER_URL=""
WORKER_SHA256=""
ACCESS_PUBLIC_KEY=""

WG_INTERFACE="wg0"
WG_ENDPOINT=""
WG_POOL="10.66.0.0/24"
WG_ADDRESS="10.66.0.1/24"
WG_LISTEN_PORT="51820"
PUBLIC_INTERFACE=""
AUTH_ADDR="127.0.0.1:9090"
INSTALL_DEPS="auto"

usage(){
cat <<'EOF'
Единая установка VPNX3 worker-ноды:

  install-worker-node.sh     --control https://CONTROL-ENDPOINT     --token ONE_TIME_WORKER_TOKEN     --name de-01     --country DE     --provider provider-a     --public-ip 203.0.113.10     --node-agent-url https://.../vpnx3-node-agent     --node-agent-sha256 SHA256     --worker-url https://.../vpnx3-vpn-worker     --worker-sha256 SHA256     --access-public-key BASE64URL_ED25519_PUBLIC_KEY     --wg-endpoint 203.0.113.10:51820

Опционально:
  --capacity 1000
  --wg-interface wg0
  --wg-pool 10.66.0.0/24
  --wg-address 10.66.0.1/24
  --wg-listen-port 51820
  --public-interface eth0
  --auth-addr 127.0.0.1:9090
  --install-deps auto|yes|no

Скрипт сначала поднимает WireGuard worker, затем Node Agent и только после этого
enrollment token используется Control Plane.
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
    --capacity) CAPACITY="${2:-}"; shift 2;;
    --node-agent-url) NODE_AGENT_URL="${2:-}"; shift 2;;
    --node-agent-sha256) NODE_AGENT_SHA256="${2:-}"; shift 2;;
    --worker-url) WORKER_URL="${2:-}"; shift 2;;
    --worker-sha256) WORKER_SHA256="${2:-}"; shift 2;;
    --access-public-key) ACCESS_PUBLIC_KEY="${2:-}"; shift 2;;
    --wg-interface) WG_INTERFACE="${2:-}"; shift 2;;
    --wg-endpoint) WG_ENDPOINT="${2:-}"; shift 2;;
    --wg-pool) WG_POOL="${2:-}"; shift 2;;
    --wg-address) WG_ADDRESS="${2:-}"; shift 2;;
    --wg-listen-port) WG_LISTEN_PORT="${2:-}"; shift 2;;
    --public-interface) PUBLIC_INTERFACE="${2:-}"; shift 2;;
    --auth-addr) AUTH_ADDR="${2:-}"; shift 2;;
    --install-deps) INSTALL_DEPS="${2:-}"; shift 2;;
    -h|--help) usage; exit 0;;
    *) echo "Неизвестный аргумент: $1" >&2; usage; exit 2;;
  esac
done

[[ "${EUID}" -eq 0 ]] || { echo "Запустите от root." >&2; exit 1; }

required=(
  CONTROL_URL ENROLLMENT_TOKEN NODE_NAME COUNTRY PROVIDER
  NODE_AGENT_URL NODE_AGENT_SHA256 WORKER_URL WORKER_SHA256
  ACCESS_PUBLIC_KEY WG_ENDPOINT
)
for name in "${required[@]}"; do
  [[ -n "${!name}" ]] || { echo "Не задан обязательный параметр: $name" >&2; exit 2; }
done

[[ "$CONTROL_URL" == https://* ]] || { echo "Control URL должен использовать HTTPS." >&2; exit 2; }

if [[ -z "$PUBLIC_IP" ]]; then
  endpoint_host="${WG_ENDPOINT%:*}"
  if [[ "$endpoint_host" =~ ^[0-9a-fA-F:.]+$ ]]; then
    PUBLIC_IP="$endpoint_host"
  fi
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
worker_installer="$script_dir/install-worker.sh"
node_installer="$script_dir/install-node.sh"

[[ -x "$worker_installer" || -f "$worker_installer" ]] || { echo "Не найден $worker_installer" >&2; exit 1; }
[[ -x "$node_installer" || -f "$node_installer" ]] || { echo "Не найден $node_installer" >&2; exit 1; }

worker_args=(
  --binary-url "$WORKER_URL"
  --sha256 "$WORKER_SHA256"
  --access-public-key "$ACCESS_PUBLIC_KEY"
  --wg-interface "$WG_INTERFACE"
  --wg-endpoint "$WG_ENDPOINT"
  --wg-pool "$WG_POOL"
  --wg-address "$WG_ADDRESS"
  --wg-listen-port "$WG_LISTEN_PORT"
  --auth-addr "$AUTH_ADDR"
  --install-deps "$INSTALL_DEPS"
)
[[ -n "$PUBLIC_INTERFACE" ]] && worker_args+=(--public-interface "$PUBLIC_INTERFACE")

echo "==> Этап 1/2: WireGuard Data Plane"
bash "$worker_installer" "${worker_args[@]}"

echo "==> Этап 2/2: Node Agent + enrollment"
node_args=(
  --control "$CONTROL_URL"
  --token "$ENROLLMENT_TOKEN"
  --name "$NODE_NAME"
  --country "$COUNTRY"
  --provider "$PROVIDER"
  --binary-url "$NODE_AGENT_URL"
  --sha256 "$NODE_AGENT_SHA256"
  --capacity "$CAPACITY"
)
[[ -n "$PUBLIC_IP" ]] && node_args+=(--public-ip "$PUBLIC_IP")

bash "$node_installer" "${node_args[@]}"

echo
echo "Worker-нода установлена."
echo "Control Plane должен увидеть её после enrollment/heartbeat."
echo "Дальше: testing -> draft -> active и публикация endpoint."
