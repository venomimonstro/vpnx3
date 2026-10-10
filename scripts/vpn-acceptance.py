#!/usr/bin/env python3
"""VPNX3 acceptance checks: REAL VPN tunnels, Telegram HTTPS, and optional RU probe.

Unlike container health checks this runs a real Xray client and (if a profile exists)
a real OpenVPN client inside a disposable Linux network namespace. It never modifies
host default routes or publishes private keys. RF probing checks TCP reachability only,
not the authenticated VPN handshake.
"""
import argparse
import importlib.util
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import signal
import subprocess
import tempfile
import sys
import time
from urllib.request import Request, urlopen
from urllib.error import HTTPError, URLError

ROOT = Path("/opt/vpnx3")
OVPN_CLIENTS = Path("/etc/openvpn/vpnx3/clients")
REALITY = ROOT / "scripts/personal-vless-manager.py"
spec = importlib.util.spec_from_file_location("vpnx3_personal_vless_manager", REALITY)
vless = importlib.util.module_from_spec(spec)
spec.loader.exec_module(vless)

def call(*args, timeout=12, check=True):
    return subprocess.run(args, capture_output=True, text=True, check=check, timeout=timeout)

def vless_tests():
    if not vless.CONFIG.exists():
        print("VLESS: SKIP (not installed)", flush=True)
        return None
    cfg = vless.load_config()
    rows = vless.current_profiles(cfg)
    modes = {}
    for row in rows:
        modes.setdefault(row["mode"], row["id"])
    if not modes:
        print("VLESS: FAIL (no profiles)", flush=True)
        return False
    success = True
    for mode, identifier in modes.items():
        for host in ("example.com", "telegram.org", "web.telegram.org"):
            try:
                response = vless.test_personal_vless(identifier, host)
                elapsed = response["local_test_ms"]
                print(f"VLESS {mode} -> HTTPS {host}: PASS ({elapsed} ms via actual tunnel)", flush=True)
            except Exception as exc:
                print(f"VLESS {mode} -> HTTPS {host}: FAIL ({type(exc).__name__}: {str(exc)[:150]})", flush=True)
                success = False
    print("VLESS caveat: these probes originate on the VPS, NOT inside Russia.", flush=True)
    return success

def summarize_openvpn_client_failure(log_file, proc):
    """Classify client handshake errors; NEVER return raw logs containing IPs/keys."""
    if log_file is None:
        return "client did not start"
    try:
        log_file.flush()
        log_file.seek(0)
        lines=log_file.read(64*1024).lower()
    except (OSError,ValueError):
        return "client logs unavailable"
    reasons=(
        ("options error", "unsupported client profile directive"),
        ("unrecognized option", "unsupported client profile directive"),
        ("permission denied", "permission denied creating tunnel or route"),
        ("cannot open tun", "TUN interface unavailable"),
        ("cannot ioctl tun", "TUN interface unavailable"),
        ("tls key negotiation failed", "TLS handshake timed out"),
        ("tls error", "TLS negotiation error"),
        ("verify error", "certificate validation failed"),
        ("certificate has expired", "certificate expired"),
        ("auth_failed", "server rejected authentication"),
        ("tls-crypt unwrap error", "tls-crypt mismatch"),
        ("network is unreachable", "client namespace cannot route to server"),
        ("no route to host", "client namespace cannot reach server"),
        ("rtnetlink answers", "route installation failed"),
        ("route addition failed", "route installation failed"),
        ("initialization sequence completed", "client initialized but expected full-tunnel routes were absent"),
    )
    for pattern,desc in reasons:
        if pattern in lines:
            return desc
    code=proc.poll() if proc else None
    return ("client exited (see journalctl -u openvpn-server@vpnx3)" if code is not None
            else "no tunnel after timeout (inspect server TLS logs)")


def openvpn_test():
    """Client namespace has an isolated route table; VPS main routes are unchanged."""
    if os.geteuid() != 0:
        print("OpenVPN: SKIP (run as root for netns)", flush=True)
        return None
    config = OVPN_CLIENTS / "vpnx3-health.ovpn"
    if not config.is_file():
        print("OpenVPN: INCOMPLETE (dedicated health certificate missing; run sudo bash scripts/install-personal-openvpn.sh install)",flush=True)
        return None
    for executable in ("ip", "openvpn", "curl"):
        if not shutil.which(executable):
            print(f"OpenVPN: SKIP ({executable} missing)", flush=True)
            return None
    # Avoid a false 23-second timeout caused by a stale OpenVPN installation.
    # Operator logs have shown 'dev tun' + missing FORWARD rules although
    # the repository installer already configures vpnx3tun0.
    server_file = Path("/etc/openvpn/server/vpnx3.conf")
    if not server_file.is_file() or not re.search(r"(?m)^dev vpnx3tun0\\s*$",server_file.read_text()):
        print("OpenVPN: FAIL (stale server configuration: run sudo bash scripts/install-personal-openvpn.sh install)",flush=True)
        return False
    if config.stat().st_size > 100*1024:
        print("OpenVPN: FAIL (oversized profile)", flush=True)
        return False
    rnd = secrets.token_hex(3)
    ns = "vnqa" + rnd
    host_if, guest_if = "vqh" + rnd, "vqg" + rnd
    # Selected test prefix. Verify no conflict before creating namespace.
    network = ipaddress.ip_network("10.254.242.0/30")
    host_ip, guest_ip = list(network.hosts())
    listed = call("ip", "-4", "route", "show", str(network), check=False)
    if listed.stdout.strip():
        print("OpenVPN: SKIP (temporary namespace subnet conflicts with server route)")
        return None
    created = False
    proc = None
    log_file = None
    log_stage = "not_started"
    try:
        call("ip", "netns", "add", ns)
        created = True
        call("ip", "link", "add", host_if, "type", "veth", "peer", "name", guest_if)
        call("ip", "link", "set", guest_if, "netns", ns)
        call("ip", "addr", "add", f"{host_ip}/30", "dev", host_if)
        call("ip", "link", "set", host_if, "up")
        call("ip", "-n", ns, "addr", "add", f"{guest_ip}/30", "dev", guest_if)
        call("ip", "-n", ns, "link", "set", "lo", "up")
        call("ip", "-n", ns, "link", "set", guest_if, "up")
        call("ip", "-n", ns, "route", "add", "default", "via", str(host_ip))
        dns = Path("/etc/netns") / ns
        dns.mkdir(parents=True, mode=0o700, exist_ok=True)
        (dns/"resolv.conf").write_text("nameserver 1.1.1.1\nnameserver 9.9.9.9\n")
        log_file = tempfile.TemporaryFile(mode="w+t", encoding="utf-8")
        proc = subprocess.Popen(
            ["ip","netns","exec",ns,"openvpn","--config",str(config),
             "--connect-retry-max","2","--verb","3"],
            stdout=log_file, stderr=subprocess.STDOUT
        )
        log_stage = "handshake"
        ready = False
        for _ in range(80):
            if proc.poll() is not None:
                break
            status = call("ip", "-n", ns, "route", "show", check=False)
            if "0.0.0.0/1" in status.stdout and "128.0.0.0/1" in status.stdout:
                ready = True
                break
            time.sleep(0.5)
        if not ready:
            detail = summarize_openvpn_client_failure(log_file, proc)
            print("OpenVPN: FAIL (client tunnel routes not established within 40s; "+detail+")",flush=True)
            print("OpenVPN: check server listener, certificate validity, tls-crypt and local namespace routes. No credentials are printed.",flush=True)
            return False
        good = True
        for host in ("example.com", "telegram.org", "web.telegram.org"):
            try:
                result = call("ip","netns","exec",ns,"curl","--ipv4","-L","-sS",
                    "--max-time","12","--connect-timeout","6",
                    "-o","/dev/null","-w","%{http_code}",
                    f"https://{host}/", timeout=15)
                status = result.stdout.strip()
                if re.fullmatch(r"[1-5][0-9]{2}", status):
                    print(f"OpenVPN -> HTTPS {host}: PASS (HTTP {status}, inside tun)", flush=True)
                else:
                    good = False
                    print(f"OpenVPN -> HTTPS {host}: FAIL (no HTTP response)", flush=True)
            except (subprocess.CalledProcessError, subprocess.TimeoutExpired):
                good = False
                print(f"OpenVPN -> HTTPS {host}: FAIL (timeout or connection error)", flush=True)
        print("OpenVPN caveat: connection is tested from a disposable namespace ON THE VPS, not from Russia.", flush=True)
        return good
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, OSError) as exc:
        print(f"OpenVPN: FAIL (isolated client test setup: {type(exc).__name__}: {str(exc)[:120]})", flush=True)
        return False
    finally:
        if proc is not None and proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=4)
        if log_file is not None:
            log_file.close()
        if created:
            call("ip", "netns", "del", ns, check=False)
            # Rarely a veth can remain after namespace teardown.
            call("ip", "link", "del", host_if, check=False)
        dns = Path("/etc/netns") / ns
        if dns.exists():
            (dns/"resolv.conf").unlink(missing_ok=True)
            dns.rmdir()

def external_ru_port_test():
    """Globalping TCP MTR from available RU probe, if any. No client creds involved."""
    if not vless.CONFIG.exists():
        print("RU probe: SKIP (no VLESS configured)")
        return None
    cfg=vless.load_config()
    ip=vless.ADDRESS
    try:
        ipaddress.ip_address(ip)
    except ValueError:
        print("RU probe: SKIP (target is not an IP)")
        return None
    port=443 if vless.HTTPS443_FLAG.is_file() and vless.local_listening(443) else int(cfg["inbounds"][0]["port"])
    body={"type":"mtr","target":ip,"locations":[{"country":"RU","limit":1}],
          "measurementOptions":{"protocol":"TCP","port":port,"packets":3}}
    headers={"Content-Type":"application/json","Accept":"application/json",
             "User-Agent":"VPNX3-operator-diagnostic/1.0"}
    try:
        req=Request("https://api.globalping.io/v1/measurements",
            data=json.dumps(body).encode(), headers=headers, method="POST")
        with urlopen(req,timeout=10) as result:
            payload=json.load(result)
        identifier=payload.get("id")
        if not identifier:
            raise ValueError("Globalping did not return a measurement ID")
        for _ in range(12):
            time.sleep(1)
            check=Request(f"https://api.globalping.io/v1/measurements/{identifier}",
                headers={"User-Agent":"VPNX3-operator-diagnostic/1.0"})
            with urlopen(check, timeout=8) as resp:
                status=json.load(resp)
            if status.get("status")!="in-progress":
                results=status.get("results",[])
                if not results:
                    print("RU probe: UNAVAILABLE (no results / possibly no probe in RU)")
                    return None
                for row in results:
                    probe=row.get("probe",{})
                    country=probe.get("country") or probe.get("location",{}).get("country")
                    if country not in ("RU","Russia","Russian Federation"):
                        print("RU probe: UNAVAILABLE (returned probe is not in Russia)")
                        return None
                print("RU probe: TCP MTR completed from RF; reachability of the service is not confirmed.")
                print("RU probe measurement ID:", identifier)
                print("RU probe: use authenticated testing from a real RF client to confirm the VPN.")
                return None
        print("RU probe: UNAVAILABLE (measurement timed out)")
        return None
    except (OSError,URLError,HTTPError,ValueError) as exc:
        print(f"RU probe: UNAVAILABLE ({type(exc).__name__}: {str(exc)[:140]})")
        return None

def main():
    parser=argparse.ArgumentParser()
    parser.add_argument("--russia", action="store_true", help="optional external TCP traceroute from RU probe")
    parser.add_argument("--skip-openvpn", action="store_true")
    args=parser.parse_args()
    result_vless=vless_tests()
    result_ovpn=None if args.skip_openvpn else openvpn_test()
    result_ru=external_ru_port_test() if args.russia else None
    print("\nACCEPTANCE: local VLESS=",result_vless,
          "local OpenVPN=",result_ovpn,"RU TCP measurement=",result_ru)
    print("An end-to-end VPN test FROM RUSSIA requires a real RF client/probe with credentials.")
    if result_vless is False or result_ovpn is False:
        return 1
    if result_vless is None or result_ovpn is None:
        print("INCOMPLETE: at least one VPN protocol was not tested through its client.")
        return 2
    return 0

if __name__=="__main__":
    sys.exit(main())
