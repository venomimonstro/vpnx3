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
    def test_shared_443_gateway_preserves_existing_admin_and_keys(self):
        compat = source("scripts/enable-vless-443.sh")
        gateway = source("scripts/enable-reality-on-443.sh")
        self.assertIn("enable-reality-on-443.sh", compat)
        self.assertIn("req.ssl_sni -i $sni", gateway)
        self.assertIn("127.0.0.1:8443 check", gateway)
        self.assertIn("127.0.0.1:443 check", gateway)
        self.assertIn("PREROUTING 1", gateway)
        self.assertIn("ExecStopPost=", gateway)
        self.assertIn("failback", gateway.lower()) if "failback" in gateway.lower() else self.assertIn("restores public", gateway.lower())
        self.assertNotIn("docker restart vpnx3-admin-proxy", gateway)
        self.assertNotIn('cp -p "$BACKUP" "$CADDY"', gateway)

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
        self.assertIn('rule_add FORWARD -i "$VPN_IF" -o "$WAN" -s "$VPN_NET" -j ACCEPT', script)
        self.assertIn('rule_add DOCKER-USER -i "$VPN_IF" -o "$WAN" -s "$VPN_NET" -j ACCEPT', script)
        self.assertIn('iptables -C FORWARD -i vpnx3tun0 -o "$WAN"', script)

    def test_openvpn_export_is_atomic(self):
        script = source("scripts/install-personal-openvpn.sh")
        self.assertIn('TEMP_PROFILE="$(mktemp "$CLIENTS/.client.XXXXXXXX.ovpn")"', script)
        self.assertIn('mv -f "$TEMP_PROFILE" "$CLIENTS/$NAME.ovpn"', script)

    def test_dedicated_443_edge_refuses_to_take_admin_port(self):
        edge = source("scripts/install-vpn-edge-443.sh")
        self.assertIn("TCP/443 is already in use", edge)
        self.assertIn('preflight', edge)
        self.assertIn('export VPNX3_PERSONAL_VLESS_PORT=443', edge)
        self.assertIn('manager_request link-ios', edge)
        main = source("scripts/install-personal-vless.sh")
        self.assertIn('PORT==443', main)

    def test_vless_issuer_refuses_stale_reality_links(self):
        manager = source("scripts/personal-vless-manager.py")
        self.assertIn("def inspect_vless_import_link(", manager)
        self.assertIn("stale_public_key", manager)
        self.assertIn("incorrect_public_endpoint", manager)
        self.assertIn("invalid_short_id", manager)
        self.assertIn("invalid VLESS import profile:", manager)

    def test_openvpn_custom_interface_has_explicit_tun_type(self):
        installer = source("scripts/install-personal-openvpn.sh")
        self.assertIn("dev vpnx3tun0\ndev-type tun\ntopology subnet", installer)
        self.assertIn('cp -p "$PREVIOUS_CONF" "$CONF"', installer)

    def test_acceptance_never_reuses_user_certificate(self):
        acceptance = source("scripts/vpn-acceptance.py")
        installer = source("scripts/install-personal-openvpn.sh")
        manager = source("scripts/personal-vless-manager.py")
        self.assertIn('OVPN_CLIENTS / "vpnx3-health.ovpn"', acceptance)
        self.assertIn('create_client "vpnx3-health"', installer)
        self.assertIn('name == "vpnx3-health"', manager)
        self.assertIn("summarize_openvpn_client_failure", acceptance)

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
