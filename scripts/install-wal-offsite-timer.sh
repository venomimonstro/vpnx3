#!/usr/bin/env bash
set -euo pipefail

BINARY="/usr/local/bin/vpnx3-wal-replicator"
ENV_FILE=""
ON_CALENDAR="*:0/2"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file) ENV_FILE="${2:-}"; shift 2;;
    --binary) BINARY="${2:-}"; shift 2;;
    --on-calendar) ON_CALENDAR="${2:-}"; shift 2;;
    -h|--help) echo "Usage: $0 --env-file /secure/vpnx3-wal.env"; exit 0;;
    *) echo "Unknown argument: $1" >&2; exit 2;;
  esac
done
[[ $EUID -eq 0 ]] || { echo "Run as root" >&2; exit 1; }
[[ -x "$BINARY" ]] || { echo "WAL replicator missing" >&2; exit 1; }
[[ -f "$ENV_FILE" ]] || { echo "Env file missing" >&2; exit 1; }
mode="$(stat -c "%a" "$ENV_FILE")"
[[ "$mode" == "400" || "$mode" == "600" ]] || { echo "Env file must be 0400/0600" >&2; exit 1; }
install -d -m 0700 /var/lib/vpnx3/wal-s3tmp /var/lib/vpnx3/backup

cat >/etc/systemd/system/vpnx3-wal-offsite.service <<EOF
[Unit]
Description=VPNX3 encrypted PostgreSQL WAL off-site replication
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=root
Group=root
EnvironmentFile=$ENV_FILE
ExecStart=$BINARY
Nice=15
IOSchedulingClass=best-effort
IOSchedulingPriority=7
PrivateTmp=true
ProtectHome=true
NoNewPrivileges=true
ProtectSystem=strict
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
ReadWritePaths=/var/lib/vpnx3/wal-s3tmp /var/lib/vpnx3/backup
EOF

cat >/etc/systemd/system/vpnx3-wal-offsite.timer <<EOF
[Unit]
Description=VPNX3 frequent encrypted WAL replication

[Timer]
OnCalendar=$ON_CALENDAR
Persistent=true
RandomizedDelaySec=20s
Unit=vpnx3-wal-offsite.service

[Install]
WantedBy=timers.target
EOF
systemctl daemon-reload
systemctl enable --now vpnx3-wal-offsite.timer
systemctl list-timers vpnx3-wal-offsite.timer --no-pager
