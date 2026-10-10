#!/usr/bin/env python3
"""Local zero-network tests for personal VLESS import profiles."""
import importlib.util
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest import TestCase, main
from unittest.mock import patch
from urllib.parse import urlsplit, parse_qs

source = Path(__file__).resolve().parents[1] / "personal-vless-manager.py"
spec = importlib.util.spec_from_file_location("vpnx3_personal_vless_manager_tested", source)
manager = importlib.util.module_from_spec(spec)
spec.loader.exec_module(manager)

VISION_ID = "11111111-1111-4111-8111-111111111111"
IOS_ID = "22222222-2222-4222-8222-222222222222"


class CompatibilityTests(TestCase):
    def setUp(self):
        self.tmp = TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path_patch = patch.object(manager, "PROFILES", Path(self.tmp.name) / "profiles.json")
        self.path_patch.start()
        self.addCleanup(self.path_patch.stop)
        self.key_patch = patch.object(manager, "existing_public_key", return_value="A" * 43)
        self.key_patch.start()
        self.addCleanup(self.key_patch.stop)
        self.conf = {"inbounds": [{
            "port": 8443,
            "settings": {"clients": [
                {"id": VISION_ID, "email": "android", "flow": "xtls-rprx-vision"},
                {"id": IOS_ID, "email": "iphone"},
            ]},
            "streamSettings": {"realitySettings": {
                "serverNames": ["dl.google.com"], "shortIds": ["abcdef0123456789"]
            }}
        }]}

    def test_android_keeps_vision(self):
        url = urlsplit(manager.uri_for(VISION_ID, "Android", self.conf))
        q = parse_qs(url.query)
        self.assertEqual(q["flow"], ["xtls-rprx-vision"])
        self.assertEqual(q["security"], ["reality"])
        self.assertEqual(q["sni"], ["dl.google.com"])

    def test_ios_does_not_require_vision(self):
        url = urlsplit(manager.uri_for(IOS_ID, "iPhone", self.conf))
        q = parse_qs(url.query)
        self.assertNotIn("flow", q)
        self.assertEqual(q["pbk"], ["A" * 43])
        self.assertEqual(q["sid"], ["abcdef0123456789"])
        self.assertEqual(q["fp"], ["chrome"])

    def test_generated_links_match_live_reality_parameters(self):
        for uid, label in ((VISION_ID, "Android"), (IOS_ID, "iPhone")):
            link = manager.uri_for(uid, label, self.conf)
            verdict = manager.inspect_vless_import_link(link, self.conf, uid, 8443)
            self.assertEqual(verdict, {"valid": True, "issues": []})

    def test_corrupted_links_cannot_be_issued(self):
        link = manager.uri_for(IOS_ID, "iPhone", self.conf)
        cases = (
            (link.replace("sid=abcdef0123456789", "sid=bad-id"), "invalid_short_id"),
            (link.replace("sni=dl.google.com", "sni=example.org"), "invalid_sni"),
            (link.replace("pbk=" + "A" * 43, "pbk=" + "B" * 43), "stale_public_key"),
            (link.replace(":8443", ":443"), "incorrect_public_endpoint"),
            (link.replace("security=reality", "security=tls"), "incorrect_security"),
            (link + "&flow=xtls-rprx-vision", "incorrect_flow"),
        )
        for broken, expected in cases:
            with self.subTest(issue=expected):
                result = manager.inspect_vless_import_link(broken, self.conf, IOS_ID, 8443)
                self.assertFalse(result["valid"])
                self.assertIn(expected, result["issues"])

    def test_modes_derive_from_server_configuration(self):
        devices = manager.current_profiles(self.conf)
        self.assertEqual([p["mode"] for p in devices], ["vision", "ios"])

    def test_no_profile_shares_wrong_uuid(self):
        with self.assertRaises(ValueError):
            manager.uri_for("33333333-3333-4333-8333-333333333333", "invalid", self.conf)


if __name__ == "__main__":
    main()
