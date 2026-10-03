#!/usr/bin/env bash
set -euo pipefail

NODE_AGENT_URL=""
NODE_AGENT_SHA256=""
INGRESS_URL=""
INGRESS_SHA256=""
CONTROL_URL=""
ENROLLMENT_TOKEN=""
ACCESS_PUBLIC_KEY=""
TLS_CERT=""
TLS_KEY=""
NODE_NAME="ingress"
NODE_PROVIDER=""
NODE_COUNTRY=""
NODE_PUBLIC_IP=""
INGRESS_ADDR=":8443"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --node-agent-url) NODE_AGENT_URL="${2:-}"; shift 2;;
    --node-agent-sha256) NODE_AGENT_SHA256="${2:-}"; shift 2;;
    --ingress-url) INGRESS_URL="${2:-}"; shift 2;;
    --ingress-sha256) INGRESS_SHA256="${2:-}"; shift 2;;
    --control-url) CONTROL_URL="${2:-}"; shift 2;;
    --enrollment-token) ENROLLMENT_TOKEN="${2:-}"; shift 2;;
    --access-public-key) ACCESS_PUBLIC_KEY="${2:-}"; shift 2;;
    --tls-cert) TLS_CERT="${2:-}"; shift 2;;
    --tls-key) TLS_KEY="${2:-}"; shift 2;;
    --node-name) NODE_NAME="${2:-}"; shift 2;;
    --provider) NODE_PROVIDER="${2:-}"; shift 2;;
    --country) NODE_COUNTRY="${2:-}"; shift 2;;
    --public-ip) NODE_PUBLIC_IP="${2:-}"; shift 2;;
    --listen) INGRESS_ADDR="${2:-}"; shift 2;;
    *) echo "Unknown argument: $1" >&2; exit 2;;
  esac
done

[[ "${EUID}" -eq 0 ]] || { echo "Run as root" >&2; exit 1; }

required=(
  NODE_AGENT_URL NODE_AGENT_SHA256 INGRESS_URL INGRESS_SHA256
  CONTROL_URL ENROLLMENT_TOKEN ACCESS_PUBLIC_KEY TLS_CERT TLS_KEY
)
for name in "${required[@]}"; do
  [[ -n "${!name}" ]] || { echo "$name is required" >&2; exit 2; }
done

[[ "$CONTROL_URL" == https://* ]] || { echo "control-url must use https" >&2; exit 2; }
[[ -f "$TLS_CERT" ]] || { echo "TLS certificate not found: $TLS_CERT" >&2; exit 2; }
[[ -f "$TLS_KEY" ]] || { echo "TLS private key not found: $TLS_KEY" >&2; exit 2; }

for cmd in curl sha256sum systemctl install useradd; do
  command -v "$cmd" >/dev/null || { echo "Missing required command: $cmd" >&2; exit 1; }
done

download_verified() {
  local url="$1" expected="$2" destination="$3"
  local tmp
  tmp="$(mktemp)"
  curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 -o "$tmp" "$url"
  local actual
  actual="$(sha256sum "$tmp" | awk '{print $1}')"
  [[ "${actual,,}" == "${expected,,}" ]] || {
    rm -f "$tmp"
    echo "SHA-256 mismatch for $url" >&2
    exit 1
  }
  install -m 0755 -o root -g root "$tmp" "$destination"
  rm -f "$tmp"
}

download_verified "$NODE_AGENT_URL" "$NODE_AGENT_SHA256" /usr/local/bin/vpnx3-node-agent
download_verified "$INGRESS_URL" "$INGRESS_SHA256" /usr/local/bin/vpnx3-ingress-proxy

id vpnx3 >/dev/null 2>&1 || useradd --system --home /var/lib/vpnx3 --shell /usr/sbin/nologin vpnx3
install -d -m 0750 -o vpnx3 -g vpnx3 /var/lib/vpnx3/agent

# TLS files may live outside /var/lib/vpnx3. The service user must be able to read them.
if ! sudo -u vpnx3 test -r "$TLS_CERT" 2>/dev/null; then
  echo "vpnx3 user cannot read TLS certificate: $TLS_CERT" >&2
  exit 1
fi
if ! sudo -u vpnx3 test -r "$TLS_KEY" 2>/dev/null; then
  echo "vpnx3 user cannot read TLS private key: $TLS_KEY" >&2
  exit 1
fi

cat >/etc/vpnx3-node-agent.env <<EOF
VPNX3_CONTROL_URL=$CONTROL_URL
VPNX3_ENROLLMENT_TOKEN=$ENROLLMENT_TOKEN
VPNX3_NODE_NAME=$NODE_NAME
VPNX3_NODE_PROVIDER=$NODE_PROVIDER
VPNX3_NODE_COUNTRY=$NODE_COUNTRY
VPNX3_NODE_PUBLIC_IP=$NODE_PUBLIC_IP
VPNX3_NODE_CAPACITY=1
VPNX3_AGENT_IDENTITY_PATH=/var/lib/vpnx3/agent/identity.json
EOF
chmod 0600 /etc/vpnx3-node-agent.env

cat >/etc/vpnx3-ingress.env <<EOF
VPNX3_ACCESS_PUBLIC_KEY=$ACCESS_PUBLIC_KEY
VPNX3_INGRESS_TLS_CERT=$TLS_CERT
VPNX3_INGRESS_TLS_KEY=$TLS_KEY
VPNX3_INGRESS_ADDR=$INGRESS_ADDR
EOF
chmod 0600 /etc/vpnx3-ingress.env

cat >/etc/systemd/system/vpnx3-node-agent.service <<'EOF'
[Unit]
Description=VPNX3 Node Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=vpnx3
Group=vpnx3
EnvironmentFile=/etc/vpnx3-node-agent.env
ExecStart=/usr/local/bin/vpnx3-node-agent
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/var/lib/vpnx3
RestrictSUIDSGID=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
EOF

cat >/etc/systemd/system/vpnx3-ingress-proxy.service <<'EOF'
[Unit]
Description=VPNX3 Browser HTTPS Ingress
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=vpnx3
Group=vpnx3
EnvironmentFile=/etc/vpnx3-ingress.env
ExecStart=/usr/local/bin/vpnx3-ingress-proxy
Restart=always
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
RestrictSUIDSGID=true
LockPersonality=true
CapabilityBoundingSet=
AmbientCapabilities=

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now vpnx3-node-agent vpnx3-ingress-proxy

echo
echo "Ingress installed."
echo "Next:"
echo "1. In Admin -> Network wait until node reaches testing."
echo "2. Approve testing: testing -> draft."
echo "3. Add endpoint: kind=ingress transport=http-connect scheme=https host=<public host> port=<TLS port>."
echo "4. Publish node: draft -> active."
echo "5. Publish signed configuration."
