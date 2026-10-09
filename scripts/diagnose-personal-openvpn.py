#!/usr/bin/env python3
"""OpenVPN server checks; optional 30-second UDP arrival probe.

Does not show client IP addresses, certificates, private keys or packet payloads.
Use --watch while reconnecting OpenVPN Connect on iPhone cellular data.
"""
import argparse
from collections import Counter
from pathlib import Path
import re
import subprocess
import sys

CONF = Path("/etc/openvpn/server/vpnx3.conf")
CLIENTS = Path("/etc/openvpn/vpnx3/clients")
NETENV = Path("/etc/openvpn/vpnx3/network.env")


def run(*args, timeout=5):
    try:
        return subprocess.run(args, capture_output=True, text=True, timeout=timeout)
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return None


def config_value(path, key):
    if not path.is_file():
        return ""
    for line in path.read_text(errors="replace").splitlines():
        cleaned = line.strip()
        if cleaned.startswith(key + " "):
            return cleaned[len(key):].strip()
        if cleaned.startswith(key + "="):
            return cleaned[len(key) + 1:].strip()
    return ""


def report():
    if not CONF.exists():
        print("FAIL: OpenVPN server config is missing")
        return None
    raw_port = config_value(CONF, "port")
    if not raw_port.isdecimal() or not (1 <= int(raw_port) <= 65535):
        print("FAIL: OpenVPN server port is invalid")
        return None
    port = int(raw_port)
    proto = config_value(CONF, "proto")
    print("OpenVPN port:", port, "protocol:", proto)
    result = run("systemctl", "is-active", "openvpn-server@vpnx3")
    active = result is not None and result.stdout.strip() == "active"
    print("OpenVPN service:", "RUNNING" if active else "STOPPED")
    result = run("ss", "-uln")
    udp_listener = bool(result and re.search(rf":{port}\s", result.stdout)) if proto.startswith("udp") else False
    print("UDP listener:", "YES" if udp_listener else "NO")
    count = len(list(CLIENTS.glob("*.ovpn"))) if CLIENTS.exists() else 0
    print("Downloaded-ready client profiles:", count)
    if count == 0:
        print("ACTION: Create a profile in /admin/ -> OpenVPN, then download .ovpn and import into OpenVPN Connect.")
    iface = config_value(NETENV, "WAN")
    if iface and not re.fullmatch(r"[A-Za-z0-9_.:-]+", iface):
        iface = ""
    for label, args in (
        ("INPUT UDP allow", ("iptables", "-C", "INPUT", "-p", "udp", "--dport", str(port), "-j", "ACCEPT")),
        ("VPN FORWARD allow", ("iptables", "-C", "FORWARD", "-s", "10.86.0.0/24", "-j", "ACCEPT")),
        ("VPN NAT masquerade", ("iptables", "-t", "nat", "-C", "POSTROUTING", "-s", "10.86.0.0/24",
                              "-o", iface, "-j", "MASQUERADE") if iface else ("false",))
    ):
        result = run(*args)
        print(label + ":", "OK" if result is not None and result.returncode == 0 else "MISSING")
    forwarding = Path("/proc/sys/net/ipv4/ip_forward").read_text().strip()
    print("IPv4 forwarding:", "ON" if forwarding == "1" else "OFF")
    journal = run("journalctl", "-u", "openvpn-server@vpnx3", "--since", "-10 minutes",
                  "--no-pager", "--output=cat", timeout=8)
    if journal and journal.returncode == 0:
        log = journal.stdout.lower()
        patterns = {
            "tls_error": "tls error",
            "certificate_verify_error": "verify error",
            "bad_packet": "authenticate/decrypt packet error",
            "client_handshake": "peer connection initiated",
            "client_auth_failed": "auth_failed",
        }
        counts = {key:log.count(pattern) for key,pattern in patterns.items()}
        print("Recent server log event counts (no IPs):", counts)
    print("Hosting provider firewall and iPhone mobile network are NOT checked from the server.")
    return port


def watch(port):
    if run("tcpdump", "--version") is None:
        print("Install tcpdump: sudo apt-get install -y tcpdump")
        return 2
    print(f"Watching incoming UDP {port} for 30 seconds.", flush=True)
    print("On iPhone disable Wi-Fi, reconnect OpenVPN Connect using cellular data.", flush=True)
    expression=f"udp and dst port {port} and not src net 127.0.0.0/8"
    response = run("timeout", "-s", "INT", "30", "tcpdump", "-i", "any",
                   "-nn", "-q", "-l", expression, timeout=38)
    if response is None:
        print("FAIL: UDP monitor could not run")
        return 2
    if response.returncode not in (0, 124):
        print("WARNING: tcpdump exited abnormally (no packet data displayed).")
    count = sum(1 for line in response.stdout.splitlines() if " > " in line)
    print("Incoming UDP packets detected (may include retries):", count)
    if count == 0:
        print("RESULT: No incoming UDP was seen. Confirm OpenVPN Connect used this server/UDP port; check provider firewall and network filtering.")
    else:
        print("RESULT: UDP reached the VPS. If the client still fails, inspect certificate/TLS errors or routes.")
    print("The monitor does not capture packet payloads or print remote IPs.")
    return 0


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--watch", action="store_true", help="monitor external UDP for 30 seconds")
    args = parser.parse_args()
    port = report()
    if port is None:
        return 2
    return watch(port) if args.watch else 0


if __name__ == "__main__":
    sys.exit(main())
