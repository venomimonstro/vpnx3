#!/usr/bin/env bash
# Dedicated VPN edge: direct Xray VLESS+REALITY on TCP 443 like the working
# Private-line-tg-claude deployment, with safe preflight and NO Marzban API.
# This script MUST NOT run on the current control VPS while Caddy owns 443.
set -Eeuo pipefail
umask 077
cd /opt/vpnx3
MODE="${1:-check}"
ROOT="/opt/vpnx3/private/personal-vless"
CFG="$ROOT/config.json"
SOCKET="$ROOT/control/manager.sock"
PORT=443
fail(){ echo "[VPN edge] ERROR: $*" >&2; exit 1; }
[[ $EUID -eq 0 ]] || fail "Run as root"
[[ -f scripts/install-personal-vless.sh ]] || fail "vpnx3 checkout not found in /opt/vpnx3"
command -v docker >/dev/null || fail "Docker is missing. Run scripts/bootstrap-control-host.sh first."
command -v python3 >/dev/null || fail "python3 required"

edge_port(){
  if [[ -f "$CFG" ]]; then
    python3 - "$CFG" <<'PY'
import json,sys
conf=json.load(open(sys.argv[1]))
print(int(conf["inbounds"][0]["port"]))
PY
  fi
}
preflight(){
  # Fail before modifying any resources. Never try to steal the admin's port.
  local existing
  existing="$(edge_port)"
  if [[ -n "$existing" && "$existing" != "$PORT" ]]; then
    fail "Existing VPN config uses TCP/$existing. No automatic migration, key rotation or port rebinding allowed."
  fi
  if [[ -z "$existing" ]] && [[ -n "$(ss -H -ltn 'sport = :443')" ]]; then
    fail "TCP/443 is already in use. Dedicated edge requires a separate IP/VPS with free 443; current admin remains untouched."
  fi
  if [[ -n "$existing" ]]; then
    docker inspect vpnx3-personal-vless >/dev/null 2>&1 || fail "Old 443 config exists but container is missing; manual recovery required"
  fi
  echo "[VPN edge] preflight OK; dedicated public 443 is available or already managed by this VPN."
}
manager_request(){
  local action="$1" name="${2:-}"
  python3 - "$SOCKET" "$action" "$name" <<'PY'
import json,socket,sys
path,action,name=sys.argv[1:]
with socket.socket(socket.AF_UNIX,socket.SOCK_STREAM) as s:
    s.settimeout(75)
    s.connect(path)
    s.sendall((json.dumps({"action":action,"name":name,"mode":"ios"})+"\n").encode())
    data=b""
    while not data.endswith(b"\n"):
        part=s.recv(65536)
        if not part: raise SystemExit("Manager disconnected")
        data+=part
        if len(data)>256*1024: raise SystemExit("Manager response too large")
result=json.loads(data)
if not result.get("ok"): raise SystemExit(str(result.get("error","Manager failure")))
if action=="create":
    print("[VPN edge] iOS profile created; key is available by explicit 'link-ios' action.")
elif action=="link-ios":
    ios=next((p for p in result.get("profiles",[]) if p.get("mode")=="ios" and p.get("uri")),None)
    if not ios: raise SystemExit("No valid iOS profile. Run 'create-ios' first.")
    # Explicit command only: secret never printed by install/status.
    print(ios["uri"])
else:
    ready=bool(result.get("running") and result.get("local_port_listening"))
    if not ready: raise SystemExit("Xray container is not ready")
    print("[VPN edge] Xray running; local port check passed (not a remote/RF test).")
PY
}
case "$MODE" in
check) preflight ;;
install)
  [[ -n "${VPNX3_EDGE_PUBLIC_ADDRESS:-}" ]] || fail "Set VPNX3_EDGE_PUBLIC_ADDRESS to this dedicated VPS's public IPv4 or DNS."
  [[ "${VPNX3_EDGE_PUBLIC_ADDRESS}" =~ ^[A-Za-z0-9.-]+$ ]] || fail "Invalid public address"
  preflight
  export VPNX3_PERSONAL_VLESS_PORT=443
  export VPNX3_PERSONAL_VLESS_IP="$VPNX3_EDGE_PUBLIC_ADDRESS"
  sudo -n true 2>/dev/null || true
  bash scripts/install-personal-vless.sh install
  if command -v ufw >/dev/null && ufw status | grep -q '^Status: active'; then
    ufw allow 443/tcp comment "VPNX3 REALITY edge"
  fi
  python3 scripts/vpn-acceptance.py --skip-openvpn
  echo "[VPN edge] Dedicated TCP/443 tested locally. Create a client via 'create-ios'."
  echo "[VPN edge] EXTERNAL MOBILE REACHABILITY STILL REQUIRES A REAL CLIENT TEST."
  ;;
create-ios)
  [[ -S "$SOCKET" ]] || fail "VPN manager unavailable. Run install first."
  # Do not generate duplicate keys on repeated invocations.
  python3 - "$SOCKET" <<'PY'
import json,socket,sys
with socket.socket(socket.AF_UNIX,socket.SOCK_STREAM) as s:
    s.settimeout(10);s.connect(sys.argv[1]);s.sendall(b'{"action":"status"}\n')
    data=b""
    while not data.endswith(b"\n"):
        chunk=s.recv(65536)
        if not chunk:raise SystemExit("manager disconnected")
        data+=chunk
        if len(data)>256*1024:raise SystemExit("response too large")
    obj=json.loads(data)
    if not obj.get("ok"):raise SystemExit("manager status failure")
    if any(p.get("mode")=="ios" for p in obj.get("profiles",[])):
        print("[VPN edge] Existing iOS profile preserved; run 'link-ios'.")
        sys.exit(10)
PY
  result=$?
  if [[ "$result" == "10" ]]; then exit 0; fi
  manager_request create iPhone
  ;;
link-ios)
  [[ -S "$SOCKET" ]] || fail "Manager unavailable"
  manager_request link-ios
  ;;
status)
  [[ -S "$SOCKET" ]] || fail "Manager unavailable"
  manager_request status
  ;;
*)
  echo "Usage: sudo bash scripts/install-vpn-edge-443.sh check|install|create-ios|link-ios|status"
  exit 2
  ;;
esac
