#!/usr/bin/env bash
set -euo pipefail

BINARY="/usr/local/bin/vpnx3-backup-replicator"
ENV_FILE=""
STATUS_FILE="/var/lib/vpnx3/backup/offsite-last-success"
ON_CALENDAR="*-*-* 04:10:00"

usage(){
cat <<'EOF'
Install VPNX3 off-site encrypted backup replication timer.

Required:
  --env-file /secure/vpnx3-offsite-backup.env

Optional:
  --binary /usr/local/bin/vpnx3-backup-replicator
  --status-file /var/lib/vpnx3/backup/offsite-last-success
  --on-calendar '*-*-* 04:10:00'

The root-only env file must contain:
  VPNX3_BACKUP_SOURCE_DIR=/srv/vpnx3-backups
  VPNX3_BACKUP_OFFSITE_STATUS_FILE=/var/lib/vpnx3/backup/offsite-last-success
  VPNX3_BACKUP_S3_ENDPOINT=https://...
  VPNX3_BACKUP_S3_REGION=...
  VPNX3_BACKUP_S3_BUCKET=...
  VPNX3_BACKUP_S3_ACCESS_KEY=...
  VPNX3_BACKUP_S3_SECRET_KEY=...
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file) ENV_FILE="${2:-}"; shift 2;;
    --binary) BINARY="${2:-}"; shift 2;;
    --status-file) STATUS_FILE="${2:-}"; shift 2;;
    --on-calendar) ON_CALENDAR="${2:-}"; shift 2;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2;;
  esac
done

[[ $EUID -eq 0 ]] || { echo "Run as root" >&2; exit 1; }
[[ -x "$BINARY" ]] || { echo "Replicator binary missing: $BINARY" >&2; exit 1; }
[[ -f "$ENV_FILE" ]] || { echo "Env file missing" >&2; exit 1; }
[[ "$STATUS_FILE" == /* ]] || { echo "status-file must be absolute" >&2; exit 2; }

mode="$(stat -c "%a" "$ENV_FILE")"
case "$mode" in
  600|400) ;;
  *) echo "Env file must have mode 0400 or 0600" >&2; exit 2;;
esac

install -d -m 0700 "$(dirname "$STATUS_FILE")"
install -d -m 0700 /var/lib/vpnx3/backup-s3tmp

cat >/etc/systemd/system/vpnx3-backup-offsite.service <<EOF
[Unit]
Description=VPNX3 verified off-site encrypted backup replication
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
ReadWritePaths=$(dirname "$STATUS_FILE") /var/lib/vpnx3/backup-s3tmp
EOF

cat >/etc/systemd/system/vpnx3-backup-offsite.timer <<EOF
[Unit]
Description=VPNX3 daily verified off-site backup replication

[Timer]
OnCalendar=$ON_CALENDAR
Persistent=true
RandomizedDelaySec=20m
Unit=vpnx3-backup-offsite.service

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now vpnx3-backup-offsite.timer
systemctl list-timers vpnx3-backup-offsite.timer --no-pager
echo "Set VPNX3_BACKUP_OFFSITE_STATUS_FILE=$STATUS_FILE on Control Plane."
