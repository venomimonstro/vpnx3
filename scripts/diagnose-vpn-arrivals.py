#!/usr/bin/env python3
"""Read-only external ingress check for VLESS/REALITY and OpenVPN.

Run on the VPS while actually reconnecting an iPhone from cellular data.
Captures packet HEADERS only on the public WAN, prints aggregate counts only,
never prints remote IP addresses, UUIDs, certificates or payloads.
"""
import argparse
import json
from pathlib import Path
import re
import shutil
import subprocess
import sys

VLESS_CONFIG = Path("/opt/vpnx3/private/personal-vless/config.json")
OVPN_CONFIG = Path("/etc/openvpn/server/vpnx3.conf")
OVPN_ENV = Path("/etc/openvpn/vpnx3/network.env")

def wan_interface():
    if OVPN_ENV.is_file():
        for line in OVPN_ENV.read_text().splitlines():
            if line.startswith("WAN="):
                name = line[4:].strip()
                if re.fullmatch(r"[A-Za-z0-9_.:-]{1,30}", name):
                    return name
    proc = subprocess.run(["ip", "-4", "route", "show", "default"],
                          text=True, capture_output=True, check=True, timeout=5)
    match = re.search(r"\bdev\s+([A-Za-z0-9_.:-]+)", proc.stdout)
    if not match:
        raise ValueError("no public IPv4 interface found")
    return match.group(1)

def ports():
    tcp = []
    if VLESS_CONFIG.exists():
        cfg = json.loads(VLESS_CONFIG.read_text())
        primary = int(cfg["inbounds"][0]["port"])
        tcp.append(primary)
    if shutil.which("docker"):
        check = subprocess.run(["docker", "port", "vpnx3-personal-vless"],
                               capture_output=True, text=True, timeout=5)
        if check.returncode == 0:
            for line in check.stdout.splitlines():
                match = re.search(r"->\s+[^:]+:(\d+)\s*$", line)
                if match:
                    tcp.append(int(match.group(1)))
    tcp = sorted({p for p in tcp if 1 <= p <= 65535})
    udp = 1194
    if OVPN_CONFIG.exists():
        match = re.search(r"(?m)^port\s+(\d+)\s*$", OVPN_CONFIG.read_text())
        if match:
            udp = int(match.group(1))
    return tcp, udp

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--seconds", type=int, default=45)
    args = parser.parse_args()
    if not 10 <= args.seconds <= 120:
        parser.error("--seconds must be 10..120")
    if not shutil.which("tcpdump"):
        print("INCOMPLETE: tcpdump not installed. Install it when apt/dpkg is free: sudo apt-get install -y tcpdump")
        return 2
    try:
        iface = wan_interface()
        tcp, udp = ports()
    except (OSError, ValueError, KeyError, IndexError, subprocess.SubprocessError) as err:
        print("FAIL: cannot resolve VPN ingress settings (" + type(err).__name__ + ")")
        return 2
    if not tcp:
        print("FAIL: no VLESS TCP ingress configured")
        return 2
    tcp_filter = " or ".join(f"dst port {port}" for port in tcp)
    expr = (f"((tcp and ({tcp_filter}) and tcp[13] & 2 != 0)"
            f" or (udp and dst port {udp}))")
    print(f"WATCH: public interface={iface} | VLESS TCP SYN={tcp} | OpenVPN UDP={udp} | {args.seconds}s", flush=True)
    print("NOW: turn iPhone Wi-Fi off. Reconnect INCY once, then OpenVPN Connect once, using cellular data.", flush=True)
    # stderr is not echoed: tcpdump capture itself contains remote public IPs.
    proc = subprocess.run(["timeout", "-s", "INT", str(args.seconds),
                           "tcpdump", "-i", iface, "-nn", "-l", "-q",
                           "-c", "10000", expr],
                          capture_output=True, text=True, timeout=args.seconds+10)
    if proc.returncode not in (0, 124):
        print("INCOMPLETE: packet capture unavailable; check tcpdump permission and interface.")
        return 2
    counts = {f"TCP/{port}": 0 for port in tcp}
    counts[f"UDP/{udp}"] = 0
    for line in proc.stdout.splitlines():
        matched = re.search(r">\s+\S+\.(\d+):", line)
        if not matched:
            continue
        port = int(matched.group(1))
        if port in tcp and "Flags [S" in line:
            counts[f"TCP/{port}"] += 1
        elif port == udp and "UDP," in line:
            counts[f"UDP/{port}"] += 1
    for name, count in counts.items():
        print(f"ARRIVAL {name}: {count} packets")
    if not any(counts.values()):
        print("NO EXTERNAL ARRIVAL during capture: check import endpoint, iPhone app, ISP or hosting-provider firewall.")
    else:
        print("SOME EXTERNAL PACKETS REACHED THE VPS. This does NOT prove authenticated VPN or usable Telegram.")
    print("Only incoming packets to the public interface were counted. No IP addresses or secrets displayed.")
    return 0

if __name__ == "__main__":
    sys.exit(main())
