#!/usr/bin/env bash
set -euo pipefail

[[ "${EUID}" -eq 0 ]] || { echo "Run as root" >&2; exit 1; }
[[ -f /etc/os-release ]] || { echo "Unsupported Linux distribution" >&2; exit 1; }
. /etc/os-release
case "$ID" in
  debian|ubuntu) ;;
  *) echo "Supported: Debian / Ubuntu only" >&2; exit 1 ;;
esac

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends ca-certificates curl git python3 openssl
if ! command -v docker >/dev/null 2>&1; then
  apt-get install -y --no-install-recommends docker.io
fi
if ! docker compose version >/dev/null 2>&1; then
  if apt-cache show docker-compose-v2 >/dev/null 2>&1; then
    apt-get install -y --no-install-recommends docker-compose-v2
  elif apt-cache show docker-compose-plugin >/dev/null 2>&1; then
    apt-get install -y --no-install-recommends docker-compose-plugin
  else
    echo "Compose v2 not available from configured apt repositories." >&2
    echo "Install the official Docker Compose v2 plugin from your trusted repository." >&2
    exit 1
  fi
fi

systemctl enable --now docker
docker info >/dev/null
docker compose version >/dev/null

ROOT="${VPNX3_INSTALL_DIR:-/opt/vpnx3}"
mkdir -p "$(dirname "$ROOT")"
if [[ -e "$ROOT" ]]; then
  [[ -d "$ROOT/.git" ]] || { echo "Existing non-Git directory: $ROOT" >&2; exit 1; }
  [[ "$(git -C "$ROOT" remote get-url origin)" == "https://github.com/venomimonstro/vpnx3.git" ]] ||
    { echo "Unexpected repository origin: $ROOT" >&2; exit 1; }
  echo "Existing checkout preserved; no automatic pull or destructive overwrite."
else
  git clone --branch main --single-branch https://github.com/venomimonstro/vpnx3.git "$ROOT"
fi

echo "VPNX3 system dependencies and checkout are ready."
if [[ ! -f "$ROOT/.env.production" ]]; then
  echo "Next: sudo bash $ROOT/scripts/install-control-production.sh prepare"
else
  echo "Production env exists. Next: review trust bundle and deploy."
fi
