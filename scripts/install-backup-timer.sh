#!/usr/bin/env bash
set -euo pipefail

ENV_FILE=""
BACKUP_SCRIPT="/usr/local/lib/vpnx3/backup-control-state.sh"
STATUS_FILE="/var/lib/vpnx3/backup/last-success"
ON_CALENDAR="*-*-* 03:20:00"

usage(){
  cat <<'EOF'
Install VPNX3 encrypted backup systemd timer.

Required:
  --env-file /secure/vpnx3-backup.env

Optional:
  --backup-script /usr/local/lib/vpnx3/backup-control-state.sh
  --status-file /var/lib/vpnx3/backup/last-success
  --on-calendar '*-*-* 03:20:00'

The env file must be root-only and contain:
  VPNX3_BACKUP_DATABASE_URL=...
  VPNX3_BACKUP_DIR=...
  VPNX3_BACKUP_AGE_RECIPIENT=age1...
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --env-file) ENV_FILE="${2:-}"; shift 2;;
    --backup-script) BACKUP_SCRIPT="${2:-}"; shift 2;;
    --status-file) STATUS_FILE="${2:-}"; shift 2;;
    --on-calendar) ON_CALENDAR="${2:-}"; shift 2;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2;;
  esac
done

[[ $EUID -eq 0 ]] || { echo "Run as root" >&2; exit 1; }
[[ -f "$ENV_FILE" ]] || { echo "env file missing" >&2; exit 2; }
[[ -f "$BACKUP_SCRIPT" ]] || { echo "backup script missing" >&2; exit 2; }

mode="$(stat -c '%a' "$ENV_FILE")"
case "$mode" in
  600|400) ;;
  *) echo "Backup env file must have mode 600 or 400" >&2; exit 2;;
esac

install -d -m 0700 "$(dirname "$STATUS_FILE")"

cat >/usr/local/lib/vpnx3/run-backup.sh <<EOF
#!/usr/bin/env bash
set -euo pipefail
set -a
source "$ENV_FILE"
set +a
"$BACKUP_SCRIPT"
tmp="${STATUS_FILE}.tmp"
date -u +%FT%TZ >"$tmp"
chmod 600 "$tmp"
mv "$tmp" "$STATUS_FILE"
EOF
chmod 0700 /usr/local/lib/vpnx3/run-backup.sh

cat >/etc/systemd/system/vpnx3-backup.service <<EOF
[Unit]
Description=VPNX3 encrypted control-plane backup
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=root
Group=root
ExecStart=/usr/local/lib/vpnx3/run-backup.sh
Nice=10
IOSchedulingClass=best-effort
IOSchedulingPriority=7
PrivateTmp=true
ProtectHome=true
NoNewPrivileges=true
EOF

cat >/etc/systemd/system/vpnx3-backup.timer <<EOF
[Unit]
Description=VPNX3 daily encrypted backup

[Timer]
OnCalendar=$ON_CALENDAR
Persistent=true
RandomizedDelaySec=20m
Unit=vpnx3-backup.service

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now vpnx3-backup.timer
systemctl list-timers vpnx3-backup.timer --no-pager
echo "Set VPNX3_BACKUP_STATUS_FILE=$STATUS_FILE for Control Plane readiness."
