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
IMAGE = "ghcr.io/xtls/xray-core:26.9.30"
CONTAINER = "vpnx3-personal-vless"
ADDRESS = os.environ.get("VPNX3_PERSONAL_VLESS_IP", "194.146.223.104")
UID_CONTROLPLANE = 65532
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
    if not LEGACY.exists():
        raise ValueError("Existing VLESS public key URI not found in client.txt")
    raw = LEGACY.read_text().strip()
    uri = urlsplit(raw)
    key = parse_qs(uri.query).get("pbk", [""])[0]
    if not re.fullmatch(r"[A-Za-z0-9_-]{43}", key):
        raise ValueError("Invalid VLESS public key URI")
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
        })
    return result


def uri_for(client_id, name, conf):
    inbound = conf["inbounds"][0]
    reality = inbound["streamSettings"]["realitySettings"]
    sni = reality["serverNames"][0]
    sid = reality["shortIds"][0]
    pbk = existing_public_key()
    port = inbound["port"]
    return (f"vless://{client_id}@{ADDRESS}:{port}"
            "?encryption=none&flow=xtls-rprx-vision&security=reality&type=tcp"
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


def get_status():
    if not CONFIG.exists():
        return {"ok": True, "configured": False, "running": False,
                "local_port_listening": False, "profiles": [], "message": "VLESS not installed"}
    config = load_config()
    profiles = current_profiles(config)
    for p in profiles:
        p["uri"] = uri_for(p["id"], p["name"], config)
    port = int(config["inbounds"][0]["port"])
    running = container_running()
    return {"ok": True, "configured": True, "running": running,
            "local_port_listening": local_listening(port) if running else False,
            "address": ADDRESS, "port": port, "profiles": profiles,
            "checked_at": dt.datetime.now(dt.timezone.utc).isoformat(),
            "connection_note": "Local port check only; internet reachability is not verified"}


def start_container(port):
    run("docker", "run", "-d", "--name", CONTAINER, "--restart", "unless-stopped",
        "--user", "0:0", "--read-only", "--cap-drop", "ALL",
        "--security-opt", "no-new-privileges", "--memory=128m", "--cpus=0.5",
        "-p", f"{port}:{port}/tcp",
        "-v", f"{CONFIG}:/etc/xray/config.json:ro",
        "--entrypoint", "xray", IMAGE, "run", "-config", "/etc/xray/config.json",
        timeout=18)


def restart_container(port):
    run("docker", "rm", "-f", CONTAINER, timeout=15, check=False)
    start_container(port)
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


def manage(action, name="", identifier=""):
    with LOCK:
        if action == "status":
            return get_status()
        if action not in {"create", "revoke"}:
            raise ValueError("Unknown action")
        if not CONFIG.exists():
            raise ValueError("Personal VLESS must be installed first")
        config = load_config()
        profiles = current_profiles(config)
        config = copy.deepcopy(config)
        clients = config["inbounds"][0]["settings"]["clients"]
        if action == "create":
            if not isinstance(name, str) or not name.strip() or len(name) > 64 or any(ord(c) < 32 for c in name):
                raise ValueError("Invalid profile name")
            if len(clients) >= 20:
                raise ValueError("Maximum 20 personal devices")
            identifier = str(uuid.uuid4())
            clients.append({"id": identifier, "flow": "xtls-rprx-vision", "email": "personal-" + identifier})
            profiles.append({"id": identifier, "name": name.strip(),
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
        self.request.settimeout(30)
        try:
            line = self.rfile.readline(8193)
            if not line or len(line) > 8192:
                raise ValueError("Request too large")
            request = json.loads(line)
            response = manage(request.get("action"), request.get("name", ""), request.get("id", ""))
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
