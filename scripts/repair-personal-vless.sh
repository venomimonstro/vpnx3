#!/usr/bin/env bash
# Upgrade the personal VLESS REALITY target safely and redeploy the admin panel.
set -euo pipefail
[[ "$EUID" == "0" ]] || { echo "Run as root" >&2; exit 1; }
cd /opt/vpnx3
python3 -m py_compile scripts/personal-vless-manager.py scripts/repair-personal-vless.py
# Avoid simultaneous writes from the online manager during key repair.
MANAGER_ACTIVE=0
if systemctl is-active --quiet vpnx3-personal-vless-manager.service; then
  MANAGER_ACTIVE=1
  systemctl stop vpnx3-personal-vless-manager.service
fi
restore_manager() {
  if [[ "$MANAGER_ACTIVE" == "1" ]]; then
    systemctl start vpnx3-personal-vless-manager.service || true
  fi
}
trap restore_manager EXIT
python3 scripts/repair-personal-vless.py --target "${VPNX3_REALITY_SNI:-dl.google.com}"
bash scripts/install-personal-vless.sh install
bash scripts/install-control-production.sh deploy
echo "[VPNX3] Repair applied. Open /admin/ -> Личный VPN to copy the updated VLESS profile."
