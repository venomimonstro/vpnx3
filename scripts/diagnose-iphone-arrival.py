#!/usr/bin/env python3
"""Diagnose whether an external iPhone reaches the VLESS port. Prints counts only.

Run this while reconnecting INCY on cellular data. No payload, IP, key or UUID is shown.
Requires tcpdump (apt-get install tcpdump). Does not change firewall or VPN config.
"""
import json
import os
from pathlib import Path
import re
import subprocess
import sys

CONFIG=Path("/opt/vpnx3/private/personal-vless/config.json")

def main():
    if os.geteuid()!=0:
        raise SystemExit("Run with sudo")
    if not CONFIG.exists():
        raise SystemExit("Personal VLESS is not installed")
    try:
        conf=json.loads(CONFIG.read_text())
        port=conf["inbounds"][0]["port"]
    except (ValueError,KeyError,IndexError) as exc:
        raise SystemExit(f"Cannot determine VLESS port: {exc}")
    if not isinstance(port,int) or not 1<=port<=65535:
        raise SystemExit("Invalid port")
    try:
        subprocess.run(["tcpdump","--version"],capture_output=True,check=True,timeout=3)
    except (FileNotFoundError,subprocess.SubprocessError):
        raise SystemExit("tcpdump not installed. Run: sudo apt-get update && sudo apt-get install -y tcpdump")
    # Capture only TCP flags and ports, NOT data. Avoid printing any source IP or secrets.
    expression=f"tcp and dst port {port} and (tcp[13] & 2 != 0)"
    print(f"Monitoring TCP SYN to {port} for 30 seconds.",flush=True)
    print("Turn off Wi-Fi on iPhone, reconnect INCY using cellular data, then open Telegram.",flush=True)
    try:
        result=subprocess.run(
            ["timeout","-s","INT","30","tcpdump","-i","any","-nn","-l","-q",
             "-c","10000",expression],
            capture_output=True,text=True,timeout=36)
    except subprocess.TimeoutExpired:
        raise SystemExit("Capture timed out")
    # Do not echo tcpdump output; report only an aggregate count.
    # TCP SYN flag is [S] or [S.] when tcpdump prints a packet summary.
    count=sum(1 for line in result.stdout.splitlines() if re.search(r"Flags \\[S\\]",line))
    if count==0:
        # tcpdump may use a variant format for SYN.
        count=sum(1 for line in result.stdout.splitlines() if "Flags [S" in line)
    print(f"External connection attempts seen (not deduplicated): {count}")
    if count==0:
        print("No SYN arrived during the test. Check INCY server address/port, mobile network and provider firewall.")
    else:
        print("SYN reached VPS. Investigate Xray handshake or iPhone client routing; this does not prove successful authentication.")
    print("No client IPs, packet payloads or VPN secrets displayed.")
    return 0

if __name__=="__main__":
    sys.exit(main())
