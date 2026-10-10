#!/usr/bin/env bash
# Acceptance checks without modifying production routes, firewall, or VPN credentials.
set -euo pipefail
cd /opt/vpnx3
[[ "${EUID}" -eq 0 ]] || { echo "Run as root: sudo bash scripts/vpn-full-check.sh" >&2; exit 1; }
echo "[VPNX3] Static configuration / syntax checks"
bash -n scripts/install-personal-vless.sh scripts/install-personal-openvpn.sh scripts/repair-personal-vless.sh scripts/enable-vless-443.sh
python3 -m py_compile scripts/personal-vless-manager.py scripts/vpn-acceptance.py \
  scripts/diagnose-personal-vless.py scripts/diagnose-personal-openvpn.py
python3 scripts/tests/test_personal_vless.py -q
python3 scripts/tests/test_vpn_deployment_safety.py -q
echo "[VPNX3] Actual tunnel acceptance checks"
python3 scripts/vpn-acceptance.py --russia
