#!/usr/bin/env bash
set -euo pipefail

CONTROL_URL=""
ENROLLMENT_TOKEN=""
NODE_NAME=""
COUNTRY=""
PROVIDER=""
PUBLIC_IP=""
NODE_AGENT_URL=""
NODE_AGENT_SHA256=""
MIRROR_URL=""
MIRROR_SHA256=""
TRUST_ROOT_PUBLIC_KEY=""
TLS_CERT=""
TLS_KEY=""
LISTEN_ADDR=":8443"

usage(){
cat <<'EOF'
Единая установка VPNX3 config-mirror ноды:

  install-config-mirror-node.sh \
    --control https://CONTROL-ENDPOINT \
    --token ONE_TIME_CONFIG_MIRROR_TOKEN \
    --name mirror-de-01 \
    --country DE \
    --provider provider-a \
    --public-ip 203.0.113.10 \
    --node-agent-url https://.../vpnx3-node-agent \
    --node-agent-sha256 SHA256 \
    --mirror-url https://.../vpnx3-config-mirror \
    --mirror-sha256 SHA256 \
    --trust-root-public-key BASE64URL_ED25519_PUBLIC_KEY \
    --tls-cert /root/tls/fullchain.pem \
    --tls-key /root/tls/privkey.pem \
    [--listen-addr :8443]

После enrollment в админке создайте endpoint:
kind=config_mirror, transport=https, scheme=https,
host=<public host/IP>, port=<listen port>, path=/api/v1/config/latest.
После проверки переведите ноду в active.
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
    --node-agent-url) NODE_AGENT_URL="${2:-}"; shift 2;;
    --node-agent-sha256) NODE_AGENT_SHA256="${2:-}"; shift 2;;
    --mirror-url) MIRROR_URL="${2:-}"; shift 2;;
    --mirror-sha256) MIRROR_SHA256="${2:-}"; shift 2;;
    --trust-root-public-key) TRUST_ROOT_PUBLIC_KEY="${2:-}"; shift 2;;
    --tls-cert) TLS_CERT="${2:-}"; shift 2;;
    --tls-key) TLS_KEY="${2:-}"; shift 2;;
    --listen-addr) LISTEN_ADDR="${2:-}"; shift 2;;
    -h|--help) usage; exit 0;;
    *) echo "Неизвестный аргумент: $1" >&2; usage; exit 2;;
  esac
done

[[ "${EUID}" -eq 0 ]] || { echo "Запустите от root." >&2; exit 1; }
required=(
  CONTROL_URL ENROLLMENT_TOKEN NODE_NAME COUNTRY PROVIDER
  NODE_AGENT_URL NODE_AGENT_SHA256 MIRROR_URL MIRROR_SHA256
  TRUST_ROOT_PUBLIC_KEY TLS_CERT TLS_KEY
)
for name in "${required[@]}";do
  [[ -n "${!name}" ]] || { echo "Не задан обязательный параметр: $name" >&2; exit 2; }
done
[[ "$CONTROL_URL" == https://* && "$NODE_AGENT_URL" == https://* && "$MIRROR_URL" == https://* ]] || {
  echo "Control и binary URL должны использовать HTTPS." >&2; exit 2;
}
[[ -f "$TLS_CERT" && -f "$TLS_KEY" ]] || { echo "TLS certificate/key не найдены." >&2; exit 1; }
for cmd in curl sha256sum systemctl install useradd;do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Не найдена команда $cmd" >&2; exit 1; }
done

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 -o "$tmp" "$MIRROR_URL"
actual="$(sha256sum "$tmp"|awk '{print $1}')"
[[ "${actual,,}" == "${MIRROR_SHA256,,}" ]] || { echo "SHA-256 config mirror не совпадает." >&2; exit 1; }

if ! id vpnx3-mirror >/dev/null 2>&1;then
  useradd --system --home /var/lib/vpnx3/config-mirror --shell /usr/sbin/nologin vpnx3-mirror
fi
install -d -m 0700 -o vpnx3-mirror -g vpnx3-mirror /var/lib/vpnx3/config-mirror
install -d -m 0750 -o root -g vpnx3-mirror /etc/vpnx3-mirror
install -m 0755 -o root -g root "$tmp" /usr/local/bin/vpnx3-config-mirror
install -m 0640 -o root -g vpnx3-mirror "$TLS_CERT" /etc/vpnx3-mirror/tls.crt
install -m 0640 -o root -g vpnx3-mirror "$TLS_KEY" /etc/vpnx3-mirror/tls.key

cat >/etc/vpnx3-mirror.env <<EOF
VPNX3_CONTROL_URL=$CONTROL_URL
VPNX3_TRUST_ROOT_PUBLIC_KEY=$TRUST_ROOT_PUBLIC_KEY
VPNX3_MIRROR_ADDR=$LISTEN_ADDR
VPNX3_MIRROR_CACHE=/var/lib/vpnx3/config-mirror/latest.json
VPNX3_MIRROR_REVOCATION_CACHE=/var/lib/vpnx3/config-mirror/revocations.json
VPNX3_MIRROR_TRUST_CACHE=/var/lib/vpnx3/config-mirror/trust-bundle.json
VPNX3_MIRROR_TLS_CERT=/etc/vpnx3-mirror/tls.crt
VPNX3_MIRROR_TLS_KEY=/etc/vpnx3-mirror/tls.key
EOF
chmod 0600 /etc/vpnx3-mirror.env

cat >/etc/systemd/system/vpnx3-config-mirror.service <<'EOF'
[Unit]
Description=VPNX3 Signed Config Mirror
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=vpnx3-mirror
Group=vpnx3-mirror
EnvironmentFile=/etc/vpnx3-mirror.env
ExecStart=/usr/local/bin/vpnx3-config-mirror
Restart=always
RestartSec=3
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
ReadWritePaths=/var/lib/vpnx3/config-mirror
ReadOnlyPaths=/etc/vpnx3-mirror

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now vpnx3-config-mirror.service

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
node_installer="$script_dir/install-node.sh"
[[ -f "$node_installer" ]] || { echo "Не найден install-node.sh" >&2; exit 1; }

node_args=(
  --control "$CONTROL_URL"
  --token "$ENROLLMENT_TOKEN"
  --name "$NODE_NAME"
  --country "$COUNTRY"
  --provider "$PROVIDER"
  --binary-url "$NODE_AGENT_URL"
  --sha256 "$NODE_AGENT_SHA256"
  --capacity 1
)
[[ -n "$PUBLIC_IP" ]] && node_args+=(--public-ip "$PUBLIC_IP")
bash "$node_installer" "${node_args[@]}"

echo
echo "Config Mirror и Node Agent установлены."
echo "Проверить:"
echo "  systemctl status vpnx3-config-mirror --no-pager"
echo "  systemctl status vpnx3-node-agent --no-pager"
echo "  curl --fail --cacert /etc/vpnx3-mirror/tls.crt https://127.0.0.1:8443/health/ready"
echo
echo "Следующий шаг: создать config_mirror endpoint в админке и опубликовать ноду."
