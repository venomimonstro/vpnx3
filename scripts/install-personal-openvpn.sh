#!/usr/bin/env bash
# Standalone OpenVPN Community 2.6 with Easy-RSA PKI. Does not modify Xray or VPNX3.
set -euo pipefail
umask 077
ACTION="${1:-help}"
NAME="${2:-}"
ROOT="/etc/openvpn/vpnx3"
PKI="$ROOT/pki"
CONF="/etc/openvpn/server/vpnx3.conf"
CLIENTS="$ROOT/clients"
PORT="${VPNX3_OPENVPN_PORT:-1194}"
ADDRESS="${VPNX3_OPENVPN_IP:-194.146.223.104}"
UNIT="openvpn-server@vpnx3"
die(){ echo "[OpenVPN] ERROR: $*" >&2; exit 1; }
[[ "$EUID" == 0 ]] || die "Run as root with sudo"
valid_name(){ [[ "$1" =~ ^[A-Za-z][A-Za-z0-9_-]{0,39}$ ]] || die "Client name must contain only ASCII letters/digits/_/- and start with a letter"; }
easy(){ (cd "$ROOT" && EASYRSA_BATCH=1 EASYRSA_PKI="$PKI" /usr/share/easy-rsa/easyrsa "$@"); }
need_install(){ [[ -f "$CONF" && -e "$PKI/ca.crt" ]] || die "Install OpenVPN first"; }
install_server(){
  [[ "$PORT" =~ ^[0-9]+$ ]] && (( PORT>1024 && PORT<=65535 )) || die "Invalid UDP port"
  [[ "$ADDRESS" =~ ^[a-zA-Z0-9.-]+$ ]] || die "Invalid public hostname/IP"
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y openvpn easy-rsa iptables
  [[ -x /usr/share/easy-rsa/easyrsa ]] || die "Easy-RSA binary missing"
  install -d -m 0700 "$ROOT" "$CLIENTS"
  install -d -m 0755 /etc/openvpn/server
  if [[ ! -e "$PKI/ca.crt" ]]; then
    easy init-pki
    EASYRSA_REQ_CN="VPNX3 OpenVPN Private CA" easy build-ca nopass
    EASYRSA_CERT_EXPIRE=825 easy build-server-full server nopass
    easy gen-crl
    openvpn --genkey tls-crypt "$ROOT/ta.key"
  fi
  [[ -f "$PKI/issued/server.crt" && -f "$PKI/private/server.key" && -f "$ROOT/ta.key" ]] || die "Missing server certificate/key"
  chmod 600 "$PKI/private/server.key" "$ROOT/ta.key"
  chmod 644 "$PKI/crl.pem"
  # All private files remain inaccessible from the Control Plane container.
  cat >"$CONF" <<EOF
port $PORT
proto udp4
dev vpnx3tun0
topology subnet
server 10.86.0.0 255.255.255.0
ca $PKI/ca.crt
cert $PKI/issued/server.crt
key $PKI/private/server.key
crl-verify $PKI/crl.pem
tls-crypt $ROOT/ta.key
dh none
ecdh-curve prime256v1
tls-version-min 1.2
data-ciphers AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305
auth SHA256
remote-cert-tls client
keepalive 10 60
persist-key
persist-tun
explicit-exit-notify 1
push "redirect-gateway def1"
push "dhcp-option DNS 1.1.1.1"
push "dhcp-option DNS 9.9.9.9"
verb 3
EOF
  # Scope forwarding to this VPN subnet; do not globally masquerade the host.
  printf 'net.ipv4.ip_forward=1\n' >/etc/sysctl.d/91-vpnx3-openvpn.conf
  sysctl -w net.ipv4.ip_forward=1 >/dev/null
  WAN="$(ip -4 route show default | awk '/default/{print $5;exit}')"
  [[ -n "$WAN" && "$WAN" =~ ^[a-zA-Z0-9_.:-]+$ ]] || die "Default IPv4 interface not detected"
  cat >"$ROOT/network.env" <<EOF
WAN=$WAN
PORT=$PORT
ADDRESS=$ADDRESS
EOF
  chmod 600 "$ROOT/network.env"
  # Idempotent firewall rules via oneshot unit, persistent over reboots.
  cat >/usr/local/sbin/vpnx3-openvpn-firewall <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
source /etc/openvpn/vpnx3/network.env
IPT=/usr/sbin/iptables
VPN_IF=vpnx3tun0
VPN_NET=10.86.0.0/24
rule_add(){
  local chain="$1"; shift
  $IPT -C "$chain" "$@" 2>/dev/null || $IPT -I "$chain" 1 "$@"
}
rule_del(){
  local chain="$1"; shift
  while $IPT -C "$chain" "$@" 2>/dev/null; do
    $IPT -D "$chain" "$@" || return 1
  done
}
has_docker_user(){ $IPT -n -L DOCKER-USER >/dev/null 2>&1; }
case "${1:-start}" in
start)
  # Old broad source-only ACCEPT rules allowed spoofed packets from any NIC.
  rule_del FORWARD -s "$VPN_NET" -j ACCEPT
  rule_del FORWARD -d "$VPN_NET" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  rule_add INPUT -p udp --dport "$PORT" -j ACCEPT
  # Interface-scoped forwarding avoids accepting internet-origin source spoofing.
  rule_add FORWARD -i "$VPN_IF" -s "$VPN_NET" -j ACCEPT
  rule_add FORWARD -o "$VPN_IF" -d "$VPN_NET" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  if has_docker_user; then
    rule_add DOCKER-USER -i "$VPN_IF" -s "$VPN_NET" -j ACCEPT
    rule_add DOCKER-USER -o "$VPN_IF" -d "$VPN_NET" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  fi
  $IPT -t nat -C POSTROUTING -s "$VPN_NET" -o "$WAN" -j MASQUERADE 2>/dev/null || $IPT -t nat -A POSTROUTING -s "$VPN_NET" -o "$WAN" -j MASQUERADE
  ;;
stop)
  rule_del INPUT -p udp --dport "$PORT" -j ACCEPT
  rule_del FORWARD -s "$VPN_NET" -j ACCEPT
  rule_del FORWARD -d "$VPN_NET" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  rule_del FORWARD -i "$VPN_IF" -s "$VPN_NET" -j ACCEPT
  rule_del FORWARD -o "$VPN_IF" -d "$VPN_NET" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  if has_docker_user; then
    rule_del DOCKER-USER -i "$VPN_IF" -s "$VPN_NET" -j ACCEPT
    rule_del DOCKER-USER -o "$VPN_IF" -d "$VPN_NET" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  fi
  while $IPT -t nat -C POSTROUTING -s "$VPN_NET" -o "$WAN" -j MASQUERADE 2>/dev/null; do
    $IPT -t nat -D POSTROUTING -s "$VPN_NET" -o "$WAN" -j MASQUERADE || break
  done
  ;;
esac
SCRIPT
  chmod 700 /usr/local/sbin/vpnx3-openvpn-firewall
  cat >/etc/systemd/system/vpnx3-openvpn-firewall.service <<'EOF'
[Unit]
Description=VPNX3 OpenVPN forwarding and NAT rules
After=network-online.target docker.service
Before=openvpn-server@vpnx3.service
Wants=network-online.target docker.service
[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/local/sbin/vpnx3-openvpn-firewall start
ExecStop=/usr/local/sbin/vpnx3-openvpn-firewall stop
[Install]
WantedBy=multi-user.target
EOF
  # A stale/expired CRL can silently reject every client even while the daemon is active.
  cat >/etc/systemd/system/vpnx3-openvpn-crl-renew.service <<'UNIT'
[Unit]
Description=VPNX3 renew OpenVPN certificate revocation list
[Service]
Type=oneshot
ExecStart=/bin/bash /opt/vpnx3/scripts/renew-openvpn-crl.sh
User=root
Group=root
NoNewPrivileges=true
PrivateTmp=true
UNIT
  cat >/etc/systemd/system/vpnx3-openvpn-crl-renew.timer <<'UNIT'
[Unit]
Description=VPNX3 automatic OpenVPN CRL renewal
[Timer]
OnCalendar=weekly
RandomizedDelaySec=3h
Persistent=true
Unit=vpnx3-openvpn-crl-renew.service
[Install]
WantedBy=timers.target
UNIT
  systemctl daemon-reload
  systemctl enable --now vpnx3-openvpn-crl-renew.timer
  if command -v ufw >/dev/null && ufw status | grep -q '^Status: active'; then
    ufw allow "$PORT/udp" comment 'VPNX3 OpenVPN'
  fi
  systemctl enable vpnx3-openvpn-firewall.service
  # RemainAfterExit=yes means enable --now will NOT re-run changed firewall rules.
  # Restart on each install to apply INPUT/FORWARD/NAT immediately and idempotently.
  systemctl restart vpnx3-openvpn-firewall.service
  systemctl is-active --quiet vpnx3-openvpn-firewall.service || die "OpenVPN firewall service failed"
  iptables -C INPUT -p udp --dport "$PORT" -j ACCEPT || die "Firewall INPUT rule not installed"
  iptables -C FORWARD -i vpnx3tun0 -s 10.86.0.0/24 -j ACCEPT || die "VPN forward rule not installed"
  iptables -t nat -C POSTROUTING -s 10.86.0.0/24 -o "$WAN" -j MASQUERADE || die "VPN NAT rule not installed"
  systemctl enable --now "$UNIT"
  systemctl restart "$UNIT"
  sleep 2
  systemctl is-active --quiet "$UNIT" || { journalctl -u "$UNIT" --no-pager -n 30; die "OpenVPN failed"; }
  ss -lun | grep -q ":$PORT " || die "OpenVPN UDP port is not listening"
  echo "[OpenVPN] Server active on UDP $PORT; firewall INPUT/FORWARD/NAT verified."
  echo "[OpenVPN] Note: provider firewall and mobile-network reachability are not verified."
  if ! compgen -G "$CLIENTS/*.ovpn" >/dev/null; then
    echo "[OpenVPN] No client profile exists. Create one in /admin/ -> OpenVPN and download .ovpn."
  else
    echo "[OpenVPN] Client profile files present: $(find "$CLIENTS" -maxdepth 1 -type f -name '*.ovpn' | wc -l)"
  fi
  echo "[OpenVPN] Diagnostics: sudo python3 scripts/diagnose-personal-openvpn.py --watch"
}
create_client(){
  need_install; valid_name "$NAME"
  source "$ROOT/network.env"
  [[ ! -f "$CLIENTS/$NAME.ovpn" ]] || die "Profile exists. Choose different name."
  [[ ! -f "$PKI/issued/$NAME.crt" ]] || die "Certificate name already used."
  EASYRSA_CERT_EXPIRE=365 easy build-client-full "$NAME" nopass
  TEMP_PROFILE="$(mktemp "$CLIENTS/.client.XXXXXXXX.ovpn")"
  trap 'rm -f "${TEMP_PROFILE:-}"' EXIT
  {
  cat <<EOF
client
dev tun
proto udp4
remote $ADDRESS $PORT
resolv-retry infinite
nobind
persist-key
persist-tun
remote-cert-tls server
auth-nocache
auth SHA256
data-ciphers AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305
verb 3
<ca>
EOF
  cat "$PKI/ca.crt"
  printf '</ca>\n<cert>\n'
  cat "$PKI/issued/$NAME.crt"
  printf '</cert>\n<key>\n'
  cat "$PKI/private/$NAME.key"
  printf '</key>\n<tls-crypt>\n'
  cat "$ROOT/ta.key"
  printf '</tls-crypt>\n'
  } >"$TEMP_PROFILE"
  chmod 600 "$TEMP_PROFILE"
  for tag in ca cert key tls-crypt; do
    grep -Fq "<$tag>" "$TEMP_PROFILE" && grep -Fq "</$tag>" "$TEMP_PROFILE" || die "Incomplete .ovpn profile"
  done
  mv -f "$TEMP_PROFILE" "$CLIENTS/$NAME.ovpn"
  trap - EXIT
  echo "[OpenVPN] Profile: $CLIENTS/$NAME.ovpn"
  echo "[OpenVPN] Import into OpenVPN Connect; protect this file as a private key."
}
revoke_client(){
  need_install;valid_name "$NAME"
  [[ -e "$PKI/issued/$NAME.crt" ]] || die "Certificate not found"
  easy revoke "$NAME"
  easy gen-crl
  chmod 644 "$PKI/crl.pem"
  rm -f "$CLIENTS/$NAME.ovpn"
  systemctl restart "$UNIT"
  echo "[OpenVPN] Revoked $NAME; profile deleted and CRL updated."
}
case "$ACTION" in
install) install_server ;;
create) create_client ;;
revoke) revoke_client ;;
status) systemctl --no-pager --full status "$UNIT";;
help|*) echo "Usage: sudo bash $0 install | create NAME | revoke NAME | status" ;;
esac
