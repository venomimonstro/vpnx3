#!/usr/bin/env bash
# Multiplex public WAN:443 using HAProxy TLS-SNI, WITHOUT touching Docker/Caddy:443.
# Public REALITY goes to existing Xray:8443; other TLS goes to Caddy:443.
# Preserves UUIDs, keys and existing admin. Manages iptables with rollback.
set -Eeuo pipefail
umask 077
cd /opt/vpnx3
ACTION="${1:-status}"
ROOT=/opt/vpnx3/private/personal-vless
STATE="$ROOT/sni443"
CONF="$STATE/haproxy.cfg"
MARKER="$ROOT/443-enabled"
UNIT=vpnx3-sni-gateway.service
NAME=vpnx3-sni443
IMAGE=haproxy:3.2.24-alpine
FRONT=10443
WAN=""
die(){ echo "[443] ERROR: $*" >&2; exit 1; }
info(){ echo "[443] $*"; }
[[ $EUID == 0 ]] || die "Run as root"
command -v iptables >/dev/null || die "iptables missing"
wan(){
  WAN="$(ip -4 route get 1.1.1.1 | awk '{for(i=1;i<=NF;i++)if($i=="dev"){print $(i+1);exit}}')"
  [[ "$WAN" =~ ^[A-Za-z0-9_.:-]{1,32}$ ]] || die "WAN not detected"
}
nat(){ iptables -t nat "$1" PREROUTING -i "$WAN" -p tcp --dport 443 -j REDIRECT --to-ports "$FRONT"; }
allow(){ iptables "$1" INPUT -i "$WAN" -p tcp --dport "$FRONT" -m conntrack --ctorigdstport 443 -j ACCEPT; }
block(){ iptables "$1" INPUT -i "$WAN" -p tcp --dport "$FRONT" -j DROP; }
gateway(){ [[ "$(docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null || :)" == true ]]; }
off(){
  wan
  while nat -C >/dev/null 2>&1; do nat -D || break; done
  while allow -C >/dev/null 2>&1; do allow -D || break; done
  while block -C >/dev/null 2>&1; do block -D || break; done
  rm -f "$MARKER"
}
on(){
  wan
  gateway || die "Gateway stopped, restoring admin"
  block -C >/dev/null 2>&1 || iptables -I INPUT 1 -i "$WAN" -p tcp --dport "$FRONT" -j DROP
  allow -C >/dev/null 2>&1 || iptables -I INPUT 1 -i "$WAN" -p tcp --dport "$FRONT" -m conntrack --ctorigdstport 443 -j ACCEPT
  first="$(iptables -t nat -S PREROUTING | grep '^-A PREROUTING ' | head -1 || :)"
  if [[ "$first" != *"--dport 443"* || "$first" != *"--to-ports $FRONT"* ]]; then
    while nat -C >/dev/null 2>&1; do nat -D || break; done
    iptables -t nat -I PREROUTING 1 -i "$WAN" -p tcp --dport 443 -j REDIRECT --to-ports "$FRONT"
  fi
  touch "$MARKER"; chmod 600 "$MARKER"
}
admin(){
  code="$(curl -k -sS --connect-timeout 3 --max-time 8 --resolve '194.146.223.104:443:127.0.0.1' -o /dev/null -w '%{http_code}' 'https://194.146.223.104/admin/' || :)"
  [[ "$code" =~ ^[234][0-9][0-9]$ ]]
}
ready(){
  gateway || return 1
  code="$(curl -k -sS --connect-timeout 3 --max-time 8 --connect-to '194.146.223.104:443:127.0.0.1:10443' -o /dev/null -w '%{http_code}' 'https://194.146.223.104/admin/' || :)"
  [[ "$code" =~ ^[234][0-9][0-9]$ ]] || return 1
  python3 - <<'PY'
import importlib.util
p="/opt/vpnx3/scripts/personal-vless-manager.py"
spec=importlib.util.spec_from_file_location("gateway_test",p)
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
answer=m.test_personal_vless(target_host="telegram.org",port_override=10443)
assert answer.get("verified"),"VLESS handshake failed"
print("[443] Local REALITY -> HTTPS telegram.org: PASS",flush=True)
PY
}
case "$ACTION" in
firewall-on) on; exit 0;;
firewall-off) off; exit 0;;
guard)
  while true; do
    gateway || die "Proxy stopped"
    admin || die "Local Caddy HTTPS down"
    on
    sleep 10
  done;;
status)
  wan
  gateway && info "SNI proxy running" || info "SNI proxy stopped"
  nat -C >/dev/null 2>&1 && info "WAN TCP/443 redirected through SNI" || info "WAN TCP/443 remains on Caddy"
  exit 0;;
disable)
  systemctl disable --now "$UNIT" >/dev/null 2>&1 || :
  off
  docker rm -f "$NAME" >/dev/null 2>&1 || :
  info "Public Caddy HTTPS/443 restored. Existing VLESS 8443 keys preserved."
  exit 0;;
enable)
  [[ -f "$ROOT/config.json" ]] || die "Install VLESS first"
  [[ -f "$ROOT/public-key.txt" ]] || die "REALITY public key missing"
  wan
  admin || die "Admin HTTPS/443 not healthy; no changes made"
  [[ "$(docker inspect -f '{{.State.Running}}' vpnx3-personal-vless 2>/dev/null || :)" == true ]] || die "Xray not running"
  info "Keeping Caddy and Xray Docker ports and keys unchanged"
  sni_port="$(python3 - "$ROOT/config.json" <<'PY'
import json,re,sys
conf=json.load(open(sys.argv[1]))
sni=conf["inbounds"][0]["streamSettings"]["realitySettings"]["serverNames"][0]
assert isinstance(sni,str) and re.fullmatch(r"[A-Za-z0-9.-]{1,200}",sni)
assert conf["inbounds"][0]["streamSettings"]["security"]=="reality"
print(sni,conf["inbounds"][0]["port"])
PY
)"
  read -r sni port <<<"$sni_port"
  [[ "$port" == 8443 ]] || die "Expected VLESS/REALITY on 8443; aborting"
  if [[ -n "$(ss -H -lnt 'sport = :10443')" ]] && ! gateway; then
    die "Port 10443 already used by another application"
  fi
  mkdir -p "$STATE"; chmod 700 "$STATE"
  cat >"$CONF" <<EOF
global
  maxconn 256
defaults
  mode tcp
  timeout connect 8s
  timeout client 1h
  timeout server 1h
frontend public_tls
  bind 0.0.0.0:10443
  tcp-request inspect-delay 5s
  tcp-request content accept if { req.ssl_hello_type 1 }
  use_backend reality if { req.ssl_sni -i $sni }
  default_backend admin
backend reality
  server xray 127.0.0.1:8443 check
backend admin
  server caddy 127.0.0.1:443 check
EOF
  chmod 644 "$CONF" # no secrets in SNI routing config
  docker image inspect "$IMAGE" >/dev/null 2>&1 || docker pull "$IMAGE"
  docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges -v "$CONF:/usr/local/etc/haproxy/haproxy.cfg:ro" --entrypoint haproxy "$IMAGE" -c -f /usr/local/etc/haproxy/haproxy.cfg
  if ! gateway; then
    docker inspect "$NAME" >/dev/null 2>&1 && docker rm -f "$NAME" >/dev/null || :
    docker run -d --name "$NAME" --restart unless-stopped --network host --read-only --cap-drop ALL --security-opt no-new-privileges --memory 96m --cpus .5 -v "$CONF:/usr/local/etc/haproxy/haproxy.cfg:ro" --entrypoint haproxy "$IMAGE" -db -f /usr/local/etc/haproxy/haproxy.cfg >/dev/null
  fi
  ready || die "Proxy health check failed BEFORE public redirection. Admin unchanged. Run: docker logs $NAME"
  cat >"/etc/systemd/system/$UNIT" <<'UNITFILE'
[Unit]
Description=VPNX3 automatic SNI multiplexer for VLESS and admin on 443
After=docker.service network-online.target
Requires=docker.service
[Service]
Type=simple
ExecStart=/bin/bash /opt/vpnx3/scripts/enable-reality-on-443.sh guard
ExecStopPost=/bin/bash /opt/vpnx3/scripts/enable-reality-on-443.sh firewall-off
Restart=always
RestartSec=10
NoNewPrivileges=true
[Install]
WantedBy=multi-user.target
UNITFILE
  systemctl daemon-reload
  systemctl enable --now "$UNIT"
  sleep 2
  if ! systemctl is-active --quiet "$UNIT" || ! nat -C; then
    systemctl disable --now "$UNIT" >/dev/null 2>&1 || :
    off
    die "Automatic rollback: gateway failed to activate"
  fi
  info "External 443 now routes REALITY to Xray and HTTPS admin to Caddy"
  info "Admin URL unchanged; existing 8443 key unchanged; new 443 link in /admin/"
  info "Rollback: sudo bash scripts/enable-reality-on-443.sh disable"
  info "RF/mobile operator access NOT verified without a real device test"
  ;;
*) echo "Usage: sudo bash scripts/enable-reality-on-443.sh enable|disable|status"; exit 2;;
esac
