#!/usr/bin/env bash
# Backward-compatible entrypoint. The previous Caddyfile rewrite was unsafe.
# New implementation never edits/recreates Caddy or changes its Docker ports.
set -Eeuo pipefail
exec /bin/bash /opt/vpnx3/scripts/enable-reality-on-443.sh "$@"
