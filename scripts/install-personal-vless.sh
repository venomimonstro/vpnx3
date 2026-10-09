#!/usr/bin/env bash
# Personal single-user VLESS + REALITY endpoint, independent from VPNX3 Control Plane.
set -euo pipefail
umask 077
ROOT="/opt/vpnx3/private/personal-vless"
IMAGE="ghcr.io/xtls/xray-core:26.9.30"
NAME="vpnx3-personal-vless"
PORT="${VPNX3_PERSONAL_VLESS_PORT:-8443}"
ADDRESS="${VPNX3_PERSONAL_VLESS_IP:-194.146.223.104}"
SNI="www.microsoft.com"
DEST="www.microsoft.com:443"
fail(){ echo "[VLESS] ERROR: $*" >&2; exit 1; }
[[ $EUID -eq 0 ]] || fail "Run as root"
[[ $PORT =~ ^[0-9]+$ ]] && (( PORT>=1024 && PORT<=65535 )) || fail "Invalid port"
command -v docker >/dev/null || fail "Docker required"
mkdir -p "$ROOT"
chmod 700 "$ROOT"
if [[ "${1:-install}" == "status" ]]; then
  docker ps --filter name="^/$NAME$"
  [[ -f "$ROOT/client.txt" ]] && cat "$ROOT/client.txt"
  exit 0
fi
[[ "${1:-install}" == "install" ]] || fail "Usage: install|status"
docker image inspect "$IMAGE" >/dev/null 2>&1 || docker pull "$IMAGE"
if [[ ! -f "$ROOT/config.json" ]]; then
  # Xray CLI produces the exact REALITY-compatible X25519 key representation.
  KEY_OUTPUT="$(docker run --rm --network none --entrypoint xray "$IMAGE" x25519)"
  PRIVATE="$(printf '%s\n' "$KEY_OUTPUT" | sed -nE 's/^(Private key|PrivateKey|Private):[[:space:]]*//p' | head -1)"
  PUBLIC="$(printf '%s\n' "$KEY_OUTPUT" | sed -nE 's/^(Public key|PublicKey|Password|Password key):[[:space:]]*//p' | head -1)"
  [[ -n "$PRIVATE" && -n "$PUBLIC" ]] || fail "Could not parse Xray x25519 output: inspect 'docker run --rm --entrypoint xray $IMAGE x25519'"
  UUID="$(python3 -c 'import uuid;print(uuid.uuid4())')"
  SHORT_ID="$(openssl rand -hex 8)"
  export PRIVATE PUBLIC UUID SHORT_ID ROOT PORT ADDRESS SNI DEST
  python3 - <<'PY'
import json,os,pathlib
root=pathlib.Path(os.environ["ROOT"])
conf={
"log":{"loglevel":"warning"},
"inbounds":[{
"listen":"0.0.0.0","port":int(os.environ["PORT"]),"protocol":"vless",
"settings":{"clients":[{"id":os.environ["UUID"],"flow":"xtls-rprx-vision","email":"personal-admin"}],"decryption":"none"},
"streamSettings":{"network":"tcp","security":"reality","realitySettings":{
"show":False,"dest":os.environ["DEST"],"xver":0,
"serverNames":[os.environ["SNI"]],"privateKey":os.environ["PRIVATE"],
"shortIds":[os.environ["SHORT_ID"]]}}
}],
"outbounds":[{"protocol":"freedom","tag":"direct"},{"protocol":"blackhole","tag":"block"}]
}
(root/"config.json").write_text(json.dumps(conf,indent=2)+"\n")
from urllib.parse import quote
uri=(f'vless://{os.environ["UUID"]}@{os.environ["ADDRESS"]}:{os.environ["PORT"]}'
'?encryption=none&flow=xtls-rprx-vision&security=reality&type=tcp'
f'&sni={quote(os.environ["SNI"])}&fp=chrome&pbk={quote(os.environ["PUBLIC"])}'
f'&sid={os.environ["SHORT_ID"]}&spx=%2F#VPNX3-Personal')
(root/"client.txt").write_text(uri+"\n")
PY
  chmod 600 "$ROOT/config.json" "$ROOT/client.txt"
fi
docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
  -v "$ROOT/config.json:/etc/xray/config.json:ro" --entrypoint xray "$IMAGE" \
  run -test -config /etc/xray/config.json
if ! docker inspect "$NAME" >/dev/null 2>&1; then
  docker run -d --name "$NAME" --restart unless-stopped \
    --read-only --cap-drop ALL --security-opt no-new-privileges \
    --memory=128m --cpus=0.5 -p "$PORT:$PORT/tcp" \
    -v "$ROOT/config.json:/etc/xray/config.json:ro" \
    --entrypoint xray "$IMAGE" run -config /etc/xray/config.json >/dev/null
else
  docker start "$NAME" >/dev/null 2>&1 || true
fi
docker ps --filter name="^/$NAME$"
echo "[VLESS] Import this profile into Happ / v2ray-compatible clients (keep secret):"
cat "$ROOT/client.txt"
echo "[VLESS] Ensure inbound TCP port $PORT is allowed by the provider firewall."
