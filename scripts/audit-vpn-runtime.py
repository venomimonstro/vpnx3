#!/usr/bin/env python3
"""Non-destructive VPS networking audit. No secrets, client IPs, or profile bodies."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path("/opt/vpnx3")
REALITY = ROOT/"private/personal-vless/config.json"
OVPN = Path("/etc/openvpn/server/vpnx3.conf")
PROFILES = Path("/etc/openvpn/vpnx3/clients")
WARN=0
FAIL=0

def run(*args, timeout=6):
    try:
        return subprocess.run(args, capture_output=True, text=True, timeout=timeout)
    except (OSError,subprocess.TimeoutExpired):
        return None

def report(label, result, detail=""):
    global WARN,FAIL
    print(f"[{result}] {label}" + (f": {detail}" if detail else ""))
    if result == "WARN": WARN += 1
    if result == "FAIL": FAIL += 1

def main():
    print("VPNX3 HOST AUDIT (read-only, no VPN credentials printed)")
    if os.geteuid() != 0:
        report("Root access", "FAIL", "use sudo")
        return 2
    up=run("sysctl","-n","net.ipv4.ip_forward")
    report("IPv4 forwarding", "OK" if up and up.stdout.strip()=="1" else "WARN")
    mounts=run("findmnt","-n","-o","FSTYPE","/")
    if mounts: report("Host root filesystem", "OK", mounts.stdout.strip()[:50])
    free=run("free","-m")
    if free and free.returncode == 0:
        for line in free.stdout.splitlines():
            if line.startswith("Mem:"):
                fields=line.split()
                try:
                    available=int(fields[6])
                    report("Free RAM", "WARN" if available<160 else "OK",f"available {available} MiB")
                except (ValueError,IndexError): pass

    if REALITY.is_file():
        try:
            conf=json.loads(REALITY.read_text())
            inbound=conf["inbounds"][0]
            port=int(inbound["port"])
            target=inbound["streamSettings"]["realitySettings"].get("target") or inbound["streamSettings"]["realitySettings"].get("dest")
            report("Xray config", "OK", f"VLESS REALITY TCP/{port}, upstream target configured={bool(target)}")
            p=run("docker","inspect","-f","{{.State.Running}}","vpnx3-personal-vless")
            report("Xray container running", "OK" if p and p.stdout.strip()=="true" else "FAIL")
            mapped=run("docker","port","vpnx3-personal-vless")
            published=bool(mapped and re.search(rf"(?m)^{port}/tcp\s+->\s+.*:{port}$",mapped.stdout))
            report("VLESS published externally", "OK" if published else "FAIL",f"TCP/{port}")
        except (ValueError,KeyError,TypeError,IndexError,OSError) as exc:
            report("Xray config", "FAIL",type(exc).__name__)
    else:
        report("Xray", "WARN","not installed")

    if OVPN.is_file():
        text=OVPN.read_text()
        port_match=re.search(r"(?m)^port\s+(\d+)$",text)
        port=int(port_match.group(1)) if port_match else 1194
        dev_match=re.search(r"(?m)^dev\s+(\S+)$",text)
        report("OpenVPN device", "OK" if dev_match and dev_match.group(1)=="vpnx3tun0" else "WARN",
               dev_match.group(1) if dev_match else "missing")
        report("OpenVPN device type", "OK" if re.search(r"(?m)^dev-type tun$",text)
               else "WARN", "required for custom vpnx3tun0 name")
        active=run("systemctl","is-active","openvpn-server@vpnx3")
        report("OpenVPN service", "OK" if active and active.stdout.strip()=="active" else "FAIL")
        ss=run("ss","-H","-uln")
        udp=bool(ss and re.search(r":"+str(port)+r"\s",ss.stdout))
        report("OpenVPN UDP listener", "OK" if udp else "FAIL",f"UDP/{port}")
        network_env=Path("/etc/openvpn/vpnx3/network.env")
        wan=""
        if network_env.exists():
            for line in network_env.read_text().splitlines():
                if line.startswith("WAN="):
                    candidate=line[4:].strip()
                    if re.fullmatch(r"[a-zA-Z0-9_.:-]+",candidate):
                        wan=candidate
                    break
        report("WAN interface", "OK" if wan else "WARN", wan if wan else "not saved")
        for desc,args in (
            ("Input firewall rule",("iptables","-C","INPUT","-p","udp","--dport",str(port),"-j","ACCEPT")),
            ("Scoped VPN forward rule",("iptables","-C","FORWARD","-i","vpnx3tun0","-o",wan,"-s","10.86.0.0/24","-j","ACCEPT") if wan else ("false",))
        ):
            p=run(*args)
            report(desc,"OK" if p and p.returncode==0 else "WARN")
        p=run("iptables","-n","-L","DOCKER-USER")
        if p and p.returncode==0:
            q=run("iptables","-C","DOCKER-USER","-i","vpnx3tun0","-o",wan,"-s","10.86.0.0/24","-j","ACCEPT") if wan else None
            report("Docker forwarding hook","OK" if q and q.returncode==0 else "WARN")
        cert=run("openssl","x509","-checkend","2592000","-noout",
                 "-in","/etc/openvpn/vpnx3/pki/issued/server.crt")
        report("Server certificate valid for 30 days","OK" if cert and cert.returncode==0 else "WARN")
        timer=run("systemctl","is-enabled","vpnx3-openvpn-crl-renew.timer")
        report("CRL renewal timer","OK" if timer and timer.stdout.strip()=="enabled" else "WARN")
        crl=run("openssl","crl","-in","/etc/openvpn/vpnx3/pki/crl.pem","-noout","-nextupdate")
        report("OpenVPN CRL readable", "OK" if crl and crl.returncode==0 else "FAIL")
        profiles=list(PROFILES.glob("*.ovpn")) if PROFILES.exists() else []
        report("Client profile files", "OK" if profiles else "WARN", f"{len(profiles)} files")
        for profile in profiles[:50]:
            # Read only the public endpoint line; NEVER print embedded key or cert.
            try:
                for line in profile.read_text().splitlines():
                    if line.startswith("remote "):
                        words=line.split()
                        if len(words)>=3 and words[2].isdigit() and int(words[2])!=port:
                            report("Profile port mismatch", "WARN",f"{profile.name}: {words[2]} != {port}")
                        break
            except (OSError,UnicodeError): report("Client profile unreadable","WARN",profile.name)
    else:
        report("OpenVPN", "WARN","not installed")

    proxy=run("docker","ps","--format","{{.Names}} {{.Ports}}")
    if proxy and proxy.returncode == 0:
        for row in proxy.stdout.splitlines():
            if "0.0.0.0:443->" in row or "127.0.0.1:443->" in row:
                report("Public TCP/443 owned by container", "WARN",row.split()[0]+
                       " — do not bind another gateway without migrating this mapping")
    marker=ROOT/"private/personal-vless/443-enabled"
    if marker.is_file():
        gateway=run("systemctl","is-active","vpnx3-sni-gateway.service")
        report("Legacy 443 gateway marker","WARN" if not gateway or gateway.stdout.strip()!="active" else "OK",
               "manually verify admin and REALITY; old gateway installer is now blocked")

    print(f"SUMMARY: FAIL={FAIL} WARN={WARN}")
    print("LOCAL AUDIT ONLY: does not prove connectivity from Russia or Telegram app on iPhone.")
    print("Next: sudo bash /opt/vpnx3/scripts/vpn-full-check.sh")
    return 1 if FAIL else 0

if __name__=="__main__": sys.exit(main())
