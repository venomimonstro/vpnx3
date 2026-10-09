#!/usr/bin/env python3
"""Repair the known broken www.microsoft.com REALITY camouflage without rotating user keys.

Backs up current files, validates target TLS, validates Xray JSON, restarts Xray,
runs a REAL VLESS+REALITY+HTTPS client test and restores the old state on failure.
Must run as root on the VPS with the host-only VLESS manager stopped.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import socket
import ssl
import subprocess
import sys

ROOT = Path("/opt/vpnx3/private/personal-vless")
SOURCE = Path("/opt/vpnx3/scripts/personal-vless-manager.py")
spec = importlib.util.spec_from_file_location("vpnx3_personal_vless_manager", SOURCE)
manager = importlib.util.module_from_spec(spec)
spec.loader.exec_module(manager)


def real_public_key(private):
    result = subprocess.run(
        ["docker", "run", "--rm", "--network", "none", "--entrypoint", "xray",
         manager.IMAGE, "x25519", "-i", private],
        capture_output=True, text=True, check=True, timeout=15
    )
    for line in result.stdout.splitlines():
        if line.partition(":")[0].strip() in ("Password", "Password (PublicKey)", "Public key", "PublicKey"):
            value = line.split(":", 1)[1].strip()
            if re.fullmatch(r"[A-Za-z0-9_-]{43}", value):
                return value
    raise ValueError("Xray could not derive the matching REALITY public key")


def preflight_target(name):
    if not re.fullmatch(r"[a-zA-Z0-9.-]{4,200}", name) or name.endswith("."):
        raise ValueError("Invalid REALITY SNI")
    ctx = ssl.create_default_context()
    ctx.set_alpn_protocols(["h2", "http/1.1"])
    with socket.create_connection((name, 443), timeout=7) as conn:
        with ctx.wrap_socket(conn, server_hostname=name) as tls:
            if tls.version() != "TLSv1.3":
                raise RuntimeError("Target does not negotiate TLS 1.3")
            if tls.selected_alpn_protocol() not in ("h2", "http/1.1"):
                raise RuntimeError("Target does not offer supported ALPN")


def restore(snapshots, port):
    for path, data in snapshots.items():
        if data is None:
            path.unlink(missing_ok=True)
        else:
            # Restore originals without exposing secrets or changing file mode.
            manager.atomic_write(path, data.decode("utf-8"))
    manager.restart_container(port)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--target", default="dl.google.com")
    parser.add_argument("--check-only", action="store_true")
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("Run as root")
    if not manager.CONFIG.exists():
        raise SystemExit("VLESS has not been installed")
    if args.check_only:
        result = manager.test_personal_vless()
        print("PASS: REALITY / VLESS / HTTPS through local Xray" if result["verified"] else "FAIL")
        return
    # Preflight BEFORE touching existing server configuration.
    preflight_target(args.target)
    original = manager.load_config()
    old_target = original["inbounds"][0]["streamSettings"]["realitySettings"]
    private = old_target["privateKey"]
    public = real_public_key(private)
    files = [manager.CONFIG, manager.PROFILES, manager.LEGACY, manager.PUBLIC]
    saved = {f: f.read_bytes() if f.exists() else None for f in files}
    port = int(original["inbounds"][0]["port"])
    try:
        profiles = manager.current_profiles(original)
        if not profiles:
            raise ValueError("No VLESS profile available for a real handshake test")
        manager.atomic_write(manager.PUBLIC, public + "\n")
        updated = json.loads(json.dumps(original))
        reality = updated["inbounds"][0]["streamSettings"]["realitySettings"]
        reality.pop("dest", None)
        reality["target"] = args.target + ":443"
        reality["serverNames"] = [args.target]
        manager.apply_change(updated, profiles)
        result = manager.test_personal_vless()
        if not result.get("verified"):
            raise RuntimeError("VLESS handshake verification did not pass")
    except Exception:
        try:
            restore(saved, port)
            print("ROLLBACK: original VLESS configuration restored", file=sys.stderr)
        except Exception as rollback_error:
            print("CRITICAL: rollback failed: " + str(rollback_error), file=sys.stderr)
        raise
    print("SUCCESS: VLESS + REALITY + HTTPS verified locally")
    print("REALITY target:", args.target)
    print("Previous UUIDs and REALITY private key preserved.")
    print("The client import URI has changed (SNI). Copy the new URI from admin panel.")
    print("This does not verify access to the VPS from another country/network.")


if __name__ == "__main__":
    main()
