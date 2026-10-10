#!/usr/bin/env python3
"""Deterministic tests for isolated OpenVPN acceptance without any live packets."""
import importlib.util
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock

MODULE = Path(__file__).resolve().parents[1] / "vpn-acceptance.py"
spec = importlib.util.spec_from_file_location("vpnx3_acceptance_tests", MODULE)
acceptance = importlib.util.module_from_spec(spec)
spec.loader.exec_module(acceptance)


class LocalClientTests(unittest.TestCase):
    def test_health_profile_uses_veth_gateway_not_public_ip(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / "health.ovpn"
            raw = ("client\nproto udp4\nremote 194.146.223.104 1194\n"
                   "<ca>\nFAKE-TEST-CERTIFICATE\n</ca>\n"
                   "<key>\nFAKE-TEST-PRIVATE-KEY\n</key>\n")
            path.write_text(raw)
            result = acceptance.isolate_openvpn_profile(path, "10.254.242.1", 1194)
            self.assertIn("remote 10.254.242.1 1194 udp4\n", result)
            self.assertNotIn("remote 194.146.223.104", result)
            self.assertIn("FAKE-TEST-PRIVATE-KEY", result)
            self.assertIn("FAKE-TEST-CERTIFICATE", result)

    def test_missing_or_duplicate_remote_rejected(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / "bad.ovpn"
            for data in ("client\n", "remote 1.2.3.4 1194\nremote 2.3.4.5 1194\n"):
                path.write_text(data)
                with self.assertRaises(ValueError):
                    acceptance.isolate_openvpn_profile(path, "10.254.242.1", 1194)

    def test_invalid_local_udp_port_rejected(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / "health.ovpn"
            path.write_text("remote 1.2.3.4 1194\n")
            for port in (0, 65536, "bad"):
                with self.assertRaises(ValueError):
                    acceptance.isolate_openvpn_profile(path, "10.254.242.1", port)

    def test_failure_classification_does_not_echo_ip_or_keys(self):
        cases = (
            ("Options error: unrecognized option found", "unsupported client profile directive"),
            ("TLS Error: TLS key negotiation failed with [AF_INET]1.2.3.4:1194", "TLS handshake timed out"),
            ("ERROR: Cannot open TUN/TAP dev /dev/net/tun", "TUN interface unavailable"),
            ("RTNETLINK answers: Network is unreachable", "client namespace cannot route"),
        )
        for logs, reason in cases:
            with self.subTest(reason=reason):
                process = Mock()
                process.poll.return_value = 1
                result = acceptance.summarize_openvpn_client_failure(io.StringIO(logs), process)
                self.assertIn(reason, result)
                self.assertNotIn("1.2.3.4", result)
                self.assertNotIn("FAKE-TEST-PRIVATE-KEY", result)


if __name__ == "__main__":
    unittest.main()
