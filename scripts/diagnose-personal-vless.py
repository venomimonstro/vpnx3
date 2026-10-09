#!/usr/bin/env python3
"""Safe local connectivity checks for VPNX3; never print client URIs or REALITY keys."""
import importlib.util
import json
from pathlib import Path
import socket
import subprocess
import sys

ROOT=Path("/opt/vpnx3")
spec=importlib.util.spec_from_file_location("personal_vless_manager", ROOT/"scripts/personal-vless-manager.py")
manager=importlib.util.module_from_spec(spec)
spec.loader.exec_module(manager)


def main():
    if not manager.CONFIG.exists():
        print("FAIL: personal VLESS config not installed")
        return 2
    cfg=manager.load_config()
    profiles=manager.current_profiles(cfg)
    port=int(cfg["inbounds"][0]["port"])
    print(f"VLESS server: {manager.ADDRESS}:{port}/tcp")
    running=manager.container_running()
    print("Xray Docker:", "RUNNING" if running else "STOPPED")
    print("Local TCP listener:", "OPEN" if running and manager.local_listening(port) else "CLOSED")
    print("Profiles:",len(profiles),"[no secrets shown]")
    published=subprocess.run(
        ["docker","port",manager.CONTAINER],capture_output=True,text=True,timeout=5)
    if published.returncode==0:
        for line in published.stdout.splitlines():
            print("Docker port:",line)
    else:
        print("Docker port: unavailable")
    print("Note: local checks do NOT prove that a mobile network can reach the VPS.")
    ok=running
    modes={}
    for row in profiles:
        modes.setdefault(row["mode"],row["id"])
    if "ios" not in modes:
        print("NOTE: no separate iPhone REALITY profile; create one via admin panel.")
    for mode,profile_id in modes.items():
        try:
            check=manager.test_personal_vless(profile_id)
            print(f"{mode}: PASS VLESS+REALITY+HTTPS via Xray, {check['local_test_ms']} ms (local)")
        except Exception as exc:
            print(f"{mode}: FAIL local handshake: {str(exc)[:190]}")
            ok=False
    try:
        p=subprocess.run(["ufw","status"],capture_output=True,text=True,timeout=5)
        if p.returncode==0:
            lines=p.stdout.splitlines()
            print("UFW:",lines[0] if lines else "unknown")
            if "Status: active" in p.stdout and f"{port}/tcp" not in p.stdout and not any(
                f"{port} " in line and "ALLOW" in line for line in lines
            ):
                print(f"WARNING: no visible UFW ALLOW rule for {port}/tcp")
        else: print("UFW: unavailable; check hosting firewall separately")
    except (FileNotFoundError,subprocess.TimeoutExpired):
        print("UFW: unavailable; check hosting firewall separately")
    print("Next: test the same profile from iPhone on Wi-Fi and mobile data.")
    if not ok:
        return 1
    print("PASS local server; an iPhone failure now needs external port/INCY logs.")
    return 0


if __name__=="__main__":
    sys.exit(main())
