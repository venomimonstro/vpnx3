#!/usr/bin/env bash
set -euo pipefail

BINARY_URL=""
BINARY_SHA256=""
CONTROL_URL=""
ENROLLMENT_TOKEN=""
NODE_NAME="build-worker"
SOURCE_REPO="https://github.com/venomimonstro/vpnx3.git"
TARGETS="android_apk,android_aab"
CLIENT_CONTROL_URL=""
CONFIG_PUBLIC_KEY=""
RELEASE_PUBLIC_KEY=""
ANDROID_SDK_ROOT_VALUE="/opt/android-sdk"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary-url) BINARY_URL="${2:-}"; shift 2;;
    --sha256) BINARY_SHA256="${2:-}"; shift 2;;
    --control-url) CONTROL_URL="${2:-}"; shift 2;;
    --enrollment-token) ENROLLMENT_TOKEN="${2:-}"; shift 2;;
    --node-name) NODE_NAME="${2:-}"; shift 2;;
    --source-repo) SOURCE_REPO="${2:-}"; shift 2;;
    --targets) TARGETS="${2:-}"; shift 2;;
    --client-control-url) CLIENT_CONTROL_URL="${2:-}"; shift 2;;
    --config-public-key) CONFIG_PUBLIC_KEY="${2:-}"; shift 2;;
    --release-public-key) RELEASE_PUBLIC_KEY="${2:-}"; shift 2;;
    --android-sdk-root) ANDROID_SDK_ROOT_VALUE="${2:-}"; shift 2;;
    *) echo "Unknown argument: $1" >&2; exit 2;;
  esac
done

[[ "${EUID}" -eq 0 ]] || { echo "Run as root" >&2; exit 1; }
[[ -n "$BINARY_URL" && -n "$BINARY_SHA256" && -n "$CONTROL_URL" && -n "$ENROLLMENT_TOKEN" ]] || {
  echo "binary-url, sha256, control-url and enrollment-token are required" >&2; exit 2;
}
for cmd in curl sha256sum systemctl install useradd git; do command -v "$cmd" >/dev/null || { echo "Missing $cmd" >&2; exit 1; }; done

tmp="$(mktemp)"; trap 'rm -f "$tmp"' EXIT
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 -o "$tmp" "$BINARY_URL"
actual="$(sha256sum "$tmp"|awk '{print $1}')"
[[ "${actual,,}" == "${BINARY_SHA256,,}" ]] || { echo "SHA-256 mismatch" >&2; exit 1; }

id vpnx3-build >/dev/null 2>&1 || useradd --system --home /var/lib/vpnx3-build-worker --shell /usr/sbin/nologin vpnx3-build
install -d -m 0750 -o vpnx3-build -g vpnx3-build /var/lib/vpnx3-build-worker /var/lib/vpnx3-build-worker/work
install -m 0755 -o root -g root "$tmp" /usr/local/bin/vpnx3-build-worker

cat >/etc/vpnx3-build-worker.env <<EOF
VPNX3_CONTROL_URL=$CONTROL_URL
VPNX3_ENROLLMENT_TOKEN=$ENROLLMENT_TOKEN
VPNX3_NODE_NAME=$NODE_NAME
VPNX3_SOURCE_REPO=$SOURCE_REPO
VPNX3_BUILD_TARGETS=$TARGETS
VPNX3_CLIENT_CONTROL_URL=$CLIENT_CONTROL_URL
VPNX3_CONFIG_PUBLIC_KEY=$CONFIG_PUBLIC_KEY
VPNX3_RELEASE_PUBLIC_KEY=$RELEASE_PUBLIC_KEY
VPNX3_AGENT_IDENTITY_PATH=/var/lib/vpnx3-build-worker/identity.json
VPNX3_BUILD_WORK_ROOT=/var/lib/vpnx3-build-worker/work
HOME=/var/lib/vpnx3-build-worker
GRADLE_USER_HOME=/var/lib/vpnx3-build-worker/.gradle
ANDROID_SDK_ROOT=$ANDROID_SDK_ROOT_VALUE
ANDROID_HOME=$ANDROID_SDK_ROOT_VALUE
EOF
chmod 0600 /etc/vpnx3-build-worker.env

cat >/etc/systemd/system/vpnx3-build-worker.service <<'EOF'
[Unit]
Description=VPNX3 Build Worker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=vpnx3-build
Group=vpnx3-build
EnvironmentFile=/etc/vpnx3-build-worker.env
ExecStart=/usr/local/bin/vpnx3-build-worker
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/var/lib/vpnx3-build-worker
RestrictSUIDSGID=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now vpnx3-build-worker
