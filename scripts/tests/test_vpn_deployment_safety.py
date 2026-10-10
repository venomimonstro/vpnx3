#!/usr/bin/env python3
"""Regression checks for security and connectivity properties of production scripts.

Static checks do not replace a real user connection test from a mobile network.
"""
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]

def source(path):
    return (ROOT / path).read_text(encoding="utf-8")

class DeploymentSafety(unittest.TestCase):
    def test_unsafe_443_topology_fails_closed(self):
        script = source("scripts/enable-vless-443.sh")
        self.assertIn('die "Shared TCP/443 cutover is disabled:', script)
        self.assertLess(script.index('die "Shared TCP/443 cutover is disabled:'),
                        script.index('[[ -f "$CADDY" ]] || die'))

    def test_vless_status_never_prints_secret_link(self):
        script = source("scripts/install-personal-vless.sh")
        status_body = script.split('if [[ "${1:-install}" == "status" ]]; then', 1)[1].split('\nfi', 1)[0]
        self.assertNotIn('cat "$ROOT/client.txt"', status_body)
        self.assertIn("authenticated admin UI", status_body)

    def test_vless_admin_endpoint_file(self):
        manager = source("scripts/personal-vless-manager.py")
        self.assertIn('ENDPOINT_FILE = ROOT / "public-address.txt"', manager)

    def test_openvpn_has_scoped_firewall_and_docker_hook(self):
        script = source("scripts/install-personal-openvpn.sh")
        self.assertIn('dev vpnx3tun0', script)
        self.assertIn('rule_add FORWARD -i "$VPN_IF" -s "$VPN_NET" -j ACCEPT', script)
        self.assertIn('rule_add DOCKER-USER -i "$VPN_IF" -s "$VPN_NET" -j ACCEPT', script)
        self.assertIn('iptables -C FORWARD -i vpnx3tun0', script)

    def test_openvpn_export_is_atomic(self):
        script = source("scripts/install-personal-openvpn.sh")
        self.assertIn('TEMP_PROFILE="$(mktemp "$CLIENTS/.client.XXXXXXXX.ovpn")"', script)
        self.assertIn('mv -f "$TEMP_PROFILE" "$CLIENTS/$NAME.ovpn"', script)

    def test_crl_renewal_timer_installed(self):
        installer = source("scripts/install-personal-openvpn.sh")
        self.assertIn('OnCalendar=weekly', installer)
        self.assertIn('vpnx3-openvpn-crl-renew.timer', installer)
        renewal = source("scripts/renew-openvpn-crl.sh")
        self.assertIn('openssl crl -in "$CRL" -noout -nextupdate', renewal)

    def test_admin_api_restricts_openvpn(self):
        routes = source("internal/httpapi/server.go")
        for method in ("GET", "POST", "DELETE"):
            self.assertIn(f'{method} /api/v1/admin/personal-openvpn', routes)
        self.assertIn('s.requireAdmin(requirePermission("admin.manage"', routes)

if __name__ == "__main__":
    unittest.main()
