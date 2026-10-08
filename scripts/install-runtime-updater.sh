#!/usr/bin/env bash
set -euo pipefail

UPDATER_URL=""; UPDATER_SHA256=""; CONTROL_URL=""; TRUST_ROOT=""
TARGET=""; CURRENT_VERSION=""; RELEASE_SOURCES=""; HEALTH_URL=""; INTERVAL="15"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --updater-url) UPDATER_URL="$2"; shift 2;;
    --updater-sha256) UPDATER_SHA256="$2"; shift 2;;
    --control-url) CONTROL_URL="$2"; shift 2;;
    --trust-root-public-key) TRUST_ROOT="$2"; shift 2;;
    --target) TARGET="$2"; shift 2;;
    --current-version) CURRENT_VERSION="$2"; shift 2;;
    --release-sources) RELEASE_SOURCES="$2"; shift 2;;
    --health-url) HEALTH_URL="$2"; shift 2;;
    --interval-minutes) INTERVAL="$2"; shift 2;;
    *) echo "Unknown argument: $1" >&2; exit 2;;
  esac
done

[[ "$EUID" -eq 0 ]] || { echo "Run as root" >&2; exit 1; }
[[ -n "$UPDATER_URL" && -n "$UPDATER_SHA256" && -n "$CONTROL_URL" && -n "$TRUST_ROOT" && -n "$TARGET" && -n "$CURRENT_VERSION" ]] || {
  echo "updater-url, updater-sha256, control-url, trust-root-public-key, target and current-version are required" >&2; exit 2;
}
[[ "$UPDATER_URL" == https://* && "$CONTROL_URL" == https://* ]] || { echo "HTTPS is required" >&2; exit 2; }
case "$TARGET" in
  node_agent_linux_amd64) MANAGED=/usr/local/bin/vpnx3-node-agent;;
  vpn_worker_linux_amd64) MANAGED=/usr/local/bin/vpnx3-worker;;
  probe_agent_linux_amd64) MANAGED=/usr/local/bin/vpnx3-probe-agent;;
  ingress_proxy_linux_amd64) MANAGED=/usr/local/bin/vpnx3-ingress-proxy;;
  config_mirror_linux_amd64) MANAGED=/usr/local/bin/vpnx3-config-mirror;;
  *) echo "Unsupported target" >&2; exit 2;;
esac
[[ -x "$MANAGED" ]] || { echo "Managed binary not found: $MANAGED" >&2; exit 1; }
[[ "$INTERVAL" =~ ^[0-9]+$ ]] && (( INTERVAL >= 5 && INTERVAL <= 1440 )) || { echo "interval must be 5..1440" >&2; exit 2; }

tmp="$(mktemp)"; trap 'rm -f "$tmp"' EXIT
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 -o "$tmp" "$UPDATER_URL"
actual="$(sha256sum "$tmp" | awk '{print $1}')"
[[ "$(printf '%s' "$actual"|tr '[:upper:]' '[:lower:]')" == "$(printf '%s' "$UPDATER_SHA256"|tr '[:upper:]' '[:lower:]')" ]] || {
  echo "Updater SHA-256 mismatch" >&2; exit 1;
}
install -m 0755 -o root -g root "$tmp" /usr/local/bin/vpnx3-runtime-updater
install -d -m 0700 -o root -g root /var/lib/vpnx3-updater

STATE="/var/lib/vpnx3-updater/$TARGET.json"
MANAGED_SHA="$(sha256sum "$MANAGED"|awk '{print $1}')"
python3 - "$STATE" "$CURRENT_VERSION" "$MANAGED_SHA" <<'PY'
import datetime,json,os,sys,tempfile
path,version,sha=sys.argv[1:]
data={"version":version,"sha256":sha.lower(),"updated_at":datetime.datetime.now(datetime.timezone.utc).isoformat().replace("+00:00","Z")}
fd,tmp=tempfile.mkstemp(prefix=".state-",dir=os.path.dirname(path))
os.fchmod(fd,0o600)
with os.fdopen(fd,"w") as f:
    json.dump(data,f,separators=(",",":")); f.flush(); os.fsync(f.fileno())
os.replace(tmp,path)
PY

install -d -m 0700 -o root -g root /etc/vpnx3-runtime-updater
cat >"/etc/vpnx3-runtime-updater/$TARGET.env" <<EOF
VPNX3_CONTROL_URL=$CONTROL_URL
VPNX3_TRUST_ROOT_PUBLIC_KEY=$TRUST_ROOT
VPNX3_RELEASE_SOURCES=$RELEASE_SOURCES
VPNX3_UPDATE_TARGET=$TARGET
VPNX3_UPDATE_STATE_PATH=$STATE
VPNX3_UPDATE_HEALTH_URL=$HEALTH_URL
EOF
chmod 0600 "/etc/vpnx3-runtime-updater/$TARGET.env"

cat >/etc/systemd/system/vpnx3-runtime-updater@.service <<'EOF'
[Unit]
Description=VPNX3 Signed Runtime Update Check
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=root
Group=root
EnvironmentFile=/etc/vpnx3-runtime-updater/%i.env
ExecStart=/usr/local/bin/vpnx3-runtime-updater
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
ReadWritePaths=/usr/local/bin /var/lib/vpnx3-updater
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
EOF

cat >/etc/systemd/system/vpnx3-runtime-updater@.timer <<EOF
[Unit]
Description=VPNX3 Signed Runtime Update Timer
[Timer]
OnBootSec=5min
OnUnitActiveSec=$INTERVAL min
RandomizedDelaySec=5min
Persistent=true
Unit=vpnx3-runtime-updater@%i.service
[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now "vpnx3-runtime-updater@$TARGET.timer"
echo "VPNX3 runtime updater enabled for $TARGET from baseline $CURRENT_VERSION"
