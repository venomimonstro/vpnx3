#!/usr/bin/env bash
# Opt-in VLESS REALITY on public 443/TCP alongside the existing HTTPS admin.
# Preserves public admin URL, existing 8443/2053 VPN ports and all private keys.
# Tests BOTH backends and automatically restores Caddy on any failed step.
set -Eeuo pipefail
umask 077
cd /opt/vpnx3
[[ $EUID -eq 0 ]] || { echo "[443] Run with sudo" >&2; exit 1; }
ACTION="${1:-status}"
PUBLIC_IP="194.146.223.104"
PROXY="/opt/vpnx3/proxy"
CADDY="$PROXY/Caddyfile"
HAPROXY="$PROXY/vpnx3-sni-443.cfg"
BACKUP="$PROXY/Caddyfile.before-sni-443"
MARKER="/opt/vpnx3/private/personal-vless/443-enabled"
SERVICE="/etc/systemd/system/vpnx3-sni-gateway.service"
ADMIN_CONTAINER="vpnx3-admin-proxy"
die(){ echo "[443] ERROR: $*" >&2; exit 1; }

check_admin(){
  curl -kfsS --connect-timeout 3 --max-time 12 \
    --resolve "$PUBLIC_IP:443:127.0.0.1" "https://$PUBLIC_IP/admin/" -o /dev/null
}
check_vpn(){
  python3 - <<'PY'
import importlib.util
p="/opt/vpnx3/scripts/personal-vless-manager.py"
spec=importlib.util.spec_from_file_location("vpnx3_sni_gateway_check",p)
m=importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
answer=m.test_personal_vless(target_host="example.com",port_override=443)
assert answer.get("verified"),"REALITY/VLESS over 443 failed"
print("[443] VLESS REALITY authenticated test: PASS",flush=True)
PY
}
restore(){
  echo "[443] Failure detected. Rolling back old admin configuration..." >&2
  trap - ERR INT TERM
  set +e
  systemctl stop vpnx3-sni-gateway.service >/dev/null 2>&1
  systemctl disable vpnx3-sni-gateway.service >/dev/null 2>&1
  if [[ -f "$BACKUP" ]]; then
    cp -p "$BACKUP" "$CADDY"
    docker restart "$ADMIN_CONTAINER" >/dev/null
    if check_admin; then
      rm -f "$BACKUP"
    else
      echo "[443] WARNING: admin health check after rollback failed; original Caddy config retained" >&2
    fi
  fi
  rm -f "$MARKER"
  echo "[443] Rollback finished; verify admin at https://$PUBLIC_IP/admin/" >&2
  exit 1
}

case "$ACTION" in
  status)
    systemctl is-active vpnx3-sni-gateway.service || true
    [[ -f "$MARKER" ]] && echo "[443] Shared ingress enabled" || echo "[443] Not enabled"
    exit 0;;
  disable)
    [[ -f "$BACKUP" ]] || die "Original Caddy config backup missing; refusing unsafe disable"
    systemctl stop vpnx3-sni-gateway.service || true
    systemctl disable vpnx3-sni-gateway.service || true
    cp -p "$BACKUP" "$CADDY"
    docker restart "$ADMIN_CONTAINER" >/dev/null
    rm -f "$MARKER"
    check_admin
    rm -f "$BACKUP"
    echo "[443] Restored original admin 443 endpoint; VPN remains on 8443/2053"
    exit 0;;
  enable) ;;
  *) echo "Usage: sudo bash scripts/enable-vless-443.sh enable|disable|status"; exit 2;;
esac

[[ -f "$CADDY" ]] || die "Caddyfile not found; no changes made"
docker inspect "$ADMIN_CONTAINER" >/dev/null 2>&1 || die "Caddy admin proxy container missing"
docker inspect vpnx3-personal-vless >/dev/null 2>&1 || die "Personal VLESS container missing"
[[ -f /opt/vpnx3/private/personal-vless/config.json ]] || die "VLESS config missing"
grep -Fq 'reverse_proxy 127.0.0.1:8080' "$CADDY" || die "Unknown Caddy config; do not overwrite"
grep -Fq 'tls internal' "$CADDY" || die "Unknown Caddy TLS config; do not overwrite"
if [[ -f "$MARKER" ]]; then
  check_admin
  check_vpn
  echo "[443] Already enabled and validated."
  exit 0
fi
[[ ! -e "$BACKUP" ]] || die "Old backup exists without enable marker; inspect previous state first"
command -v curl >/dev/null || die "curl required"
command -v python3 >/dev/null || die "python3 required"
if ! command -v haproxy >/dev/null; then
  DEBIAN_FRONTEND=noninteractive apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y haproxy
fi
SNI="$(python3 - <<'PY'
import json
p="/opt/vpnx3/private/personal-vless/config.json"
r=json.load(open(p))["inbounds"][0]["streamSettings"]["realitySettings"]
print(r["serverNames"][0])
PY
)"
[[ "$SNI" =~ ^[A-Za-z0-9.-]+$ ]] || die "Invalid REALITY SNI"
install -d -m 0750 "$PROXY"
cat >"$HAPROXY" <<EOF
global
  maxconn 512
  user haproxy
  group haproxy
defaults
  mode tcp
  timeout connect 8s
  timeout client 2m
  timeout server 2m
frontend vpnx3_public_tls
  bind 0.0.0.0:443
  tcp-request inspect-delay 4s
  tcp-request content accept if { req.ssl_hello_type 1 }
  use_backend xray_reality if { req.ssl_sni -i $SNI }
  default_backend vpnx3_admin_https
backend xray_reality
  server local_xray 127.0.0.1:8443 check
backend vpnx3_admin_https
  server local_caddy 127.0.0.1:9443 check
EOF
haproxy -c -f "$HAPROXY" || die "HAProxy config invalid"
# Reject unsupported Caddy container layouts. Existing container must use the Caddyfile path.
docker inspect -f '{{range .Mounts}}{{println .Source}}{{end}}' "$ADMIN_CONTAINER" | grep -Fxq "$CADDY" || die "Caddy container does not mount expected Caddyfile"
cp -p "$CADDY" "$BACKUP"
trap restore ERR INT TERM
cat >"$CADDY" <<EOF
https://$PUBLIC_IP:9443 {
  bind 127.0.0.1
  tls internal
  reverse_proxy 127.0.0.1:8080
}
EOF
docker restart "$ADMIN_CONTAINER" >/dev/null
curl -kfsS --connect-timeout 3 --max-time 12 \
  --resolve "$PUBLIC_IP:9443:127.0.0.1" "https://$PUBLIC_IP:9443/admin/" -o /dev/null
cat >"$SERVICE" <<EOF
[Unit]
Description=VPNX3 shared HTTPS and VLESS REALITY 443/TCP ingress
After=docker.service
Requires=docker.service
[Service]
Type=simple
ExecStart=/usr/sbin/haproxy -db -f $HAPROXY
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now vpnx3-sni-gateway.service
for i in 1 2 3 4 5; do
  systemctl is-active --quiet vpnx3-sni-gateway.service && break
  sleep 1
done
systemctl is-active --quiet vpnx3-sni-gateway.service
check_admin
check_vpn
touch "$MARKER"
chmod 600 "$MARKER"
trap - ERR INT TERM
echo "[443] SUCCESS: Admin unchanged at https://$PUBLIC_IP/admin/"
echo "[443] VLESS REALITY shares public TCP/443 (existing UUIDs, SNI, and keys preserved)."
echo "[443] Refresh /admin/ -> Личный VPN -> copy new HTTPS 443 key."
echo "[443] External RF connectivity is not established by localhost tests."
