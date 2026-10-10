#!/usr/bin/env python3
"""Host-only manager for the already installed personal Xray container.

The control plane receives neither /var/run/docker.sock nor the REALITY private key.
Only the trusted administrator API can send commands over a group-restricted Unix socket.
"""
import copy
import datetime as dt
import json
import os
from pathlib import Path
import re
import ssl
import time
import socket
import socketserver
import struct
import subprocess
import tempfile
import threading
import uuid
from urllib.parse import parse_qs, quote, urlsplit

ROOT = Path("/opt/vpnx3/private/personal-vless")
CONTROL = ROOT / "control"
SOCKET = CONTROL / "manager.sock"
CONFIG = ROOT / "config.json"
PROFILES = ROOT / "profiles.json"
LEGACY = ROOT / "client.txt"
PUBLIC = ROOT / "public-key.txt"
IMAGE = "ghcr.io/xtls/xray-core:26.9.30"
CONTAINER = "vpnx3-personal-vless"
ENDPOINT_FILE = ROOT / "public-address.txt"
ADDRESS = os.environ.get("VPNX3_PERSONAL_VLESS_IP") or (
    ENDPOINT_FILE.read_text().strip() if ENDPOINT_FILE.is_file()
    else "194.146.223.104"
)
UID_CONTROLPLANE = 65532
ALT_PORT = 2053
HTTPS443_FLAG = ROOT / "443-enabled"
LOCK = threading.RLock()
UUID_RE = re.compile(r"^[a-fA-F0-9]{8}-(?:[a-fA-F0-9]{4}-){3}[a-fA-F0-9]{12}$")


def run(*args, timeout=20, check=True):
    return subprocess.run(args, check=check, timeout=timeout, capture_output=True, text=True)


def atomic_write(path, data):
    fd, tmp = tempfile.mkstemp(prefix="." + path.name + "-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as stream:
            os.fchmod(stream.fileno(), 0o600)
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(tmp, path)
    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)


def load_config():
    return json.loads(CONFIG.read_text())


def existing_public_key():
    if PUBLIC.exists():
        key = PUBLIC.read_text().strip()
    elif LEGACY.exists():
        raw = LEGACY.read_text().strip()
        uri = urlsplit(raw)
        key = parse_qs(uri.query).get("pbk", [""])[0]
        if re.fullmatch(r"[A-Za-z0-9_-]{43}", key):
            atomic_write(PUBLIC, key + "\n")
    else:
        raise ValueError("VLESS public key not found")
    if not re.fullmatch(r"[A-Za-z0-9_-]{43}", key):
        raise ValueError("Invalid VLESS public key")
    return key


def current_profiles(conf):
    clients = conf["inbounds"][0]["settings"]["clients"]
    stored = {}
    if PROFILES.exists():
        rows = json.loads(PROFILES.read_text()).get("profiles", [])
        stored = {p["id"]: p for p in rows}
    result = []
    for item in clients:
        identifier = item["id"]
        row = stored.get(identifier, {})
        result.append({
            "id": identifier,
            "name": row.get("name", "Личный ключ" if item.get("email") == "personal-admin" else item.get("email", "Устройство")),
            "created_at": row.get("created_at"),
            "mode": "vision" if item.get("flow") == "xtls-rprx-vision" else "ios",
        })
    return result


def uri_for(client_id, name, conf, external_port=None):
    inbound = conf["inbounds"][0]
    reality = inbound["streamSettings"]["realitySettings"]
    sni = reality["serverNames"][0]
    sid = reality["shortIds"][0]
    pbk = existing_public_key()
    port = external_port or inbound["port"]
    client = next((item for item in inbound["settings"]["clients"] if item["id"] == client_id), None)
    if client is None:
        raise ValueError("VLESS profile not found")
    flow = client.get("flow", "")
    flow_param = "&flow=xtls-rprx-vision" if flow == "xtls-rprx-vision" else ""
    return (f"vless://{client_id}@{ADDRESS}:{port}"
            f"?encryption=none{flow_param}&security=reality&type=tcp"
            f"&sni={quote(sni)}&fp=chrome&pbk={quote(pbk)}"
            f"&sid={sid}&spx=%2F#{quote(name)}")


def container_running():
    result = run("docker", "inspect", "-f", "{{.State.Running}}", CONTAINER, timeout=4, check=False)
    return result.returncode == 0 and result.stdout.strip() == "true"


def local_listening(port):
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=0.7):
            return True
    except OSError:
        return False


def alternate_port_published():
    info = run("docker", "port", CONTAINER, timeout=4, check=False)
    if info.returncode != 0:
        return False
    return any(line.strip().endswith(f":{ALT_PORT}") for line in info.stdout.splitlines())


def port_free_for_fallback():
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            sock.bind(("0.0.0.0", ALT_PORT))
        return True
    except OSError:
        return False


def enable_alternate_port():
    config = load_config()
    port = int(config["inbounds"][0]["port"])
    if ALT_PORT == port or alternate_port_published():
        return get_status()
    if not port_free_for_fallback():
        raise RuntimeError(f"alternate_port_{ALT_PORT}_already_used")
    try:
        restart_container(port, alternate=True)
        if not alternate_port_published() or not container_running():
            raise RuntimeError("alternate port not published")
    except Exception as error:
        try:
            restart_container(port, alternate=False)
        except Exception as rollback_error:
            raise RuntimeError(f"alternate port failed and rollback failed: {rollback_error}") from error
        raise RuntimeError(f"alternate port unavailable; original listener restored: {error}") from error
    return get_status()


def get_status():
    if not CONFIG.exists():
        return {"ok": True, "configured": False, "running": False,
                "local_port_listening": False, "profiles": [], "message": "VLESS not installed"}
    config = load_config()
    profiles = current_profiles(config)
    alternative = alternate_port_published()
    tls443 = (HTTPS443_FLAG.is_file() and
              run("systemctl", "is-active", "--quiet", "vpnx3-sni-gateway.service",
                  timeout=3, check=False).returncode == 0 and local_listening(443))
    for p in profiles:
        p["uri"] = uri_for(p["id"], p["name"], config)
        if tls443:
            p["https_uri"] = uri_for(p["id"], p["name"], config, 443)
        if alternative:
            p["alternate_uri"] = uri_for(p["id"], p["name"], config, ALT_PORT)
    port = int(config["inbounds"][0]["port"])
    running = container_running()
    return {"ok": True, "configured": True, "running": running,
            "local_port_listening": local_listening(port) if running else False,
            "address": ADDRESS, "port": port, "alternate_port": ALT_PORT if alternative else None,
            "https_443_available": tls443, "profiles": profiles,
            "checked_at": dt.datetime.now(dt.timezone.utc).isoformat(),
            "connection_note": "Local port check only; internet reachability is not verified"}


def start_container(port, alternate=False):
    ports = ["-p", f"{port}:{port}/tcp"]
    if alternate and port != ALT_PORT:
        ports += ["-p", f"{ALT_PORT}:{port}/tcp"]
    run("docker", "run", "-d", "--name", CONTAINER, "--restart", "unless-stopped",
        "--user", "0:0", "--read-only", "--cap-drop", "ALL",
        "--security-opt", "no-new-privileges", "--memory=192m", "--cpu-shares=512",
        *ports,
        "-v", f"{CONFIG}:/etc/xray/config.json:ro",
        "--entrypoint", "xray", IMAGE, "run", "-config", "/etc/xray/config.json",
        timeout=18)


def restart_container(port, alternate=None):
    if alternate is None:
        alternate = alternate_port_published()
    run("docker", "rm", "-f", CONTAINER, timeout=15, check=False)
    start_container(port, alternate=alternate)
    if not container_running():
        raise RuntimeError("Xray failed to start")


def apply_change(conf, profiles):
    original = CONFIG.read_text()
    candidate = ROOT / "config.candidate.json"
    port = int(conf["inbounds"][0]["port"])
    atomic_write(candidate, json.dumps(conf, ensure_ascii=False, indent=2) + "\n")
    try:
        run("docker", "run", "--rm", "--network", "none", "--user", "0:0",
            "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
            "-v", f"{candidate}:/etc/xray/config.json:ro",
            "--entrypoint", "xray", IMAGE, "run", "-test", "-config",
            "/etc/xray/config.json", timeout=18)
        atomic_write(CONFIG, candidate.read_text())
        try:
            restart_container(port)
        except Exception:
            atomic_write(CONFIG, original)
            restart_container(port)
            raise
        atomic_write(PROFILES, json.dumps({"profiles": profiles}, ensure_ascii=False, indent=2) + "\n")
        if profiles:
            atomic_write(LEGACY, uri_for(profiles[0]["id"], profiles[0]["name"], conf) + "\n")
        else:
            if LEGACY.exists():
                LEGACY.unlink()
    finally:
        candidate.unlink(missing_ok=True)



def _recv_exact(conn, length):
    data = b""
    while len(data) < length:
        part = conn.recv(length - len(data))
        if not part:
            raise RuntimeError("SOCKS connection closed during REALITY test")
        data += part
    return data


def test_personal_vless(profile_id="", target_host="example.com", port_override=None):
    """Perform actual Xray client REALITY handshake and HTTPS through the local server.

    This is an end-to-end localhost test, not a claim of external reachability.
    It does not require container Docker socket exposure to the web application.
    """
    if target_host not in ("example.com", "telegram.org", "web.telegram.org", "core.telegram.org"):
        raise ValueError("unsupported acceptance-test target")
    conf = load_config()
    profiles = current_profiles(conf)
    if not profiles:
        raise ValueError("Create a VLESS profile before testing")
    first = next((p for p in profiles if p["id"] == profile_id), None) if profile_id else profiles[0]
    if first is None:
        raise ValueError("VLESS profile not found")
    started_at = time.monotonic()
    inbound = conf["inbounds"][0]
    r = inbound["streamSettings"]["realitySettings"]
    pbk = existing_public_key()
    # Reserve a random loopback port. Xray binds immediately after the reservation closes.
    with socket.socket() as reservation:
        reservation.bind(("127.0.0.1", 0))
        local_port = reservation.getsockname()[1]
    ident = uuid.uuid4().hex[:12]
    name = "vpnx3-vless-check-" + ident
    path = ROOT / ("check-" + ident + ".json")
    client = {
        "log": {"loglevel": "warning"},
        "inbounds": [{"listen": "127.0.0.1", "port": local_port,
                      "protocol": "socks", "settings": {"auth": "noauth", "udp": False}}],
        "outbounds": [{
            "protocol": "vless",
            "settings": {"vnext": [{
                "address": "127.0.0.1", "port": port_override or inbound["port"],
                "users": [{"id": first["id"], "encryption": "none",
                           "flow": "xtls-rprx-vision" if first["mode"] == "vision" else ""}]
            }]},
            "streamSettings": {"network": "tcp", "security": "reality",
                               "realitySettings": {
                                   "serverName": r["serverNames"][0],
                                   "fingerprint": "chrome",
                                   "password": pbk,
                                   "shortId": r["shortIds"][0],
                                   "spiderX": "/"
                               }}
        }]
    }
    atomic_write(path, json.dumps(client, separators=(",", ":")) + "\n")
    started = False
    try:
        run("docker", "run", "-d", "--name", name, "--network", "host",
            "--user", "0:0", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
            "--memory=128m", "--cpus=0.5",
            "-v", f"{path}:/etc/xray/config.json:ro",
            "--entrypoint", "xray", IMAGE, "run", "-config", "/etc/xray/config.json",
            timeout=12)
        started = True
        last = None
        for attempt in range(20):
            try:
                conn = socket.create_connection(("127.0.0.1", local_port), timeout=1)
                break
            except OSError as error:
                last = error
                time.sleep(0.25)
        else:
            raise RuntimeError(f"Xray test client could not start: {last}")
        with conn:
            conn.settimeout(12)
            conn.sendall(b"\x05\x01\x00")
            if _recv_exact(conn, 2) != b"\x05\x00":
                raise RuntimeError("SOCKS test negotiation failed")
            host = target_host.encode("ascii")
            conn.sendall(b"\x05\x01\x00\x03" + bytes([len(host)]) + host + (443).to_bytes(2, "big"))
            head = _recv_exact(conn, 4)
            if head[1] != 0:
                raise RuntimeError(f"SOCKS test connect failed: code {head[1]}")
            if head[3] == 1:
                _recv_exact(conn, 6)
            elif head[3] == 4:
                _recv_exact(conn, 18)
            elif head[3] == 3:
                _recv_exact(conn, _recv_exact(conn, 1)[0] + 2)
            else:
                raise RuntimeError("Invalid SOCKS test response")
            tls = ssl.create_default_context()
            with tls.wrap_socket(conn, server_hostname=target_host) as secure:
                secure.settimeout(12)
                secure.sendall(("HEAD / HTTP/1.1\r\nHost: "+target_host+"\r\nConnection: close\r\n\r\n").encode("ascii"))
                data = secure.recv(256)
                if not data.startswith(b"HTTP/"):
                    raise RuntimeError("No valid HTTPS response over VLESS tunnel")
        return {"ok": True, "verified": True, "check": "REALITY+VLESS+HTTPS",
                "profile_id": first["id"], "mode": first["mode"], "target": target_host,
                "local_test_ms": round((time.monotonic() - started_at) * 1000),
                "note": "Подключение проверено локально с реальным Xray-клиентом; внешняя доступность IP не проверяется"}
    finally:
        if started:
            run("docker", "rm", "-f", name, check=False, timeout=12)
        path.unlink(missing_ok=True)



OPENVPN_SCRIPT = "/opt/vpnx3/scripts/install-personal-openvpn.sh"
OPENVPN_PROFILES = Path("/etc/openvpn/vpnx3/clients")
OPENVPN_SERVER = Path("/etc/openvpn/server/vpnx3.conf")
OPENVPN_NAME = re.compile(r"^[A-Za-z][A-Za-z0-9_-]{0,39}$")


def manage_openvpn(action, name=""):
    if action == "openvpn_status":
        installed = OPENVPN_SERVER.exists()
        active = run("systemctl", "is-active", "--quiet", "openvpn-server@vpnx3",
                     check=False, timeout=5).returncode == 0 if installed else False
        names = sorted(path.stem for path in OPENVPN_PROFILES.glob("*.ovpn")
                       if OPENVPN_NAME.fullmatch(path.stem) and path.stem != "vpnx3-health") if installed else []
        port = 1194
        if installed:
            for line in OPENVPN_SERVER.read_text().splitlines():
                if line.startswith("port "):
                    candidate = line.split(None, 1)[1].strip()
                    if candidate.isdecimal() and 1 <= int(candidate) <= 65535:
                        port = int(candidate)
                    break
        port_list = run("ss", "-H", "-uln", timeout=4, check=False) if installed else None
        listening = bool(port_list and port_list.returncode == 0 and
                         re.search(r":" + str(port) + r"\s", port_list.stdout))
        certificate = run("openssl", "x509", "-checkend", "0", "-noout",
                          "-in", "/etc/openvpn/vpnx3/pki/issued/server.crt",
                          timeout=4, check=False) if installed else None
        certificate_valid = certificate is not None and certificate.returncode == 0
        crl_valid = False
        if installed:
            crl_info = run("openssl", "crl", "-noout", "-nextupdate",
                           "-in", "/etc/openvpn/vpnx3/pki/crl.pem",
                           timeout=4, check=False)
            if crl_info.returncode == 0 and "=" in crl_info.stdout:
                try:
                    from email.utils import parsedate_to_datetime
                    expiry = parsedate_to_datetime(crl_info.stdout.split("=", 1)[1].strip())
                    crl_valid = expiry > dt.datetime.now(dt.timezone.utc)
                except (ValueError, TypeError):
                    pass
        return {"ok": True, "installed": installed, "running": active,
                "listener_open": listening, "certificate_valid": certificate_valid,
                "crl_valid": crl_valid,
                "profiles": [{"name": n} for n in names], "port": port,
                "protocol": "udp", "ready": active and listening and certificate_valid and crl_valid}
    if action not in {"openvpn_create", "openvpn_download", "openvpn_revoke"}:
        raise ValueError("invalid OpenVPN action")
    if not OPENVPN_NAME.fullmatch(name) or name == "vpnx3-health":
        raise ValueError("invalid or reserved OpenVPN profile name")
    if not OPENVPN_SERVER.exists():
        raise ValueError("OpenVPN server not installed; run install-personal-openvpn.sh install")
    destination = OPENVPN_PROFILES / (name + ".ovpn")
    if action == "openvpn_create":
        if destination.exists():
            raise ValueError("OpenVPN profile already exists")
        run("bash", OPENVPN_SCRIPT, "create", name, timeout=90)
        return {"ok": True, "name": name}
    if not destination.is_file():
        raise ValueError("OpenVPN profile not found")
    if action == "openvpn_download":
        raw = destination.read_bytes()
        if len(raw) > 96*1024:
            raise ValueError("OpenVPN profile too large")
        import base64
        return {"ok": True, "name": name, "content_base64": base64.b64encode(raw).decode("ascii")}
    run("bash", OPENVPN_SCRIPT, "revoke", name, timeout=90)
    return {"ok": True, "name": name}


def manage(action, name="", identifier="", mode="vision"):
    with LOCK:
        if action.startswith("openvpn_"):
            return manage_openvpn(action, name)
        if action == "status":
            return get_status()
        if action == "enable_alternate_port":
            return enable_alternate_port()
        if action == "check":
            return test_personal_vless(identifier)
        if action not in {"create", "revoke"}:
            raise ValueError("Unknown action")
        if not CONFIG.exists():
            raise ValueError("Personal VLESS must be installed first")
        config = load_config()
        # Preserve the REALITY public key before any legacy URI can be revoked.
        existing_public_key()
        profiles = current_profiles(config)
        config = copy.deepcopy(config)
        clients = config["inbounds"][0]["settings"]["clients"]
        if action == "create":
            if not isinstance(name, str) or not name.strip() or len(name) > 64 or any(ord(c) < 32 for c in name):
                raise ValueError("Invalid profile name")
            if mode not in ("vision", "ios"):
                raise ValueError("Invalid VLESS compatibility mode")
            if len(clients) >= 20:
                raise ValueError("Maximum 20 personal devices")
            identifier = str(uuid.uuid4())
            user = {"id": identifier, "email": "personal-" + identifier}
            if mode == "vision":
                user["flow"] = "xtls-rprx-vision"
            clients.append(user)
            profiles.append({"id": identifier, "name": name.strip(), "mode": mode,
                             "created_at": dt.datetime.now(dt.timezone.utc).isoformat()})
        else:
            if not isinstance(identifier, str) or not UUID_RE.fullmatch(identifier):
                raise ValueError("Invalid profile ID")
            if not any(item["id"] == identifier for item in clients):
                raise ValueError("Profile not found")
            clients[:] = [item for item in clients if item["id"] != identifier]
            profiles = [p for p in profiles if p["id"] != identifier]
        apply_change(config, profiles)
        response = get_status()
        response["updated_id"] = identifier
        return response


class Handler(socketserver.StreamRequestHandler):
    def handle(self):
        # Docker bridge/controlplane UID is 65532 on normal rootful Linux Docker.
        if hasattr(socket, "SO_PEERCRED"):
            credentials = self.request.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12)
            _pid, uid, _gid = struct.unpack("3i", credentials)
            if uid not in (UID_CONTROLPLANE, 0):
                return
        self.request.settimeout(100)
        try:
            line = self.rfile.readline(8193)
            if not line or len(line) > 8192:
                raise ValueError("Request too large")
            request = json.loads(line)
            response = manage(request.get("action"), request.get("name", ""), request.get("id", ""), request.get("mode", "vision"))
        except Exception as exc:
            response = {"ok": False, "error": str(exc)[:220]}
        self.wfile.write((json.dumps(response, ensure_ascii=False, separators=(",", ":")) + "\n").encode())


def main():
    ROOT.mkdir(mode=0o700, parents=True, exist_ok=True)
    CONTROL.mkdir(mode=0o750, parents=True, exist_ok=True)
    os.chown(CONTROL, 0, UID_CONTROLPLANE)
    os.chmod(CONTROL, 0o750)
    SOCKET.unlink(missing_ok=True)
    with socketserver.UnixStreamServer(str(SOCKET), Handler) as server:
        os.chown(SOCKET, 0, UID_CONTROLPLANE)
        os.chmod(SOCKET, 0o660)
        print("Personal VLESS manager ready on local Unix socket", flush=True)
        server.serve_forever(poll_interval=0.2)


if __name__ == "__main__":
    main()
