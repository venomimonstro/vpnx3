#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${VPNX3_BACKUP_FILE:?VPNX3_BACKUP_FILE is required}"
: "${VPNX3_BACKUP_AGE_IDENTITY:?VPNX3_BACKUP_AGE_IDENTITY is required}"
: "${VPNX3_RESTORE_DATABASE_URL:?VPNX3_RESTORE_DATABASE_URL is required}"
: "${VPNX3_RESTORE_CONFIRM:?Set VPNX3_RESTORE_CONFIRM=RESTORE_ISOLATED_DATABASE}"

[[ "$VPNX3_RESTORE_CONFIRM" == "RESTORE_ISOLATED_DATABASE" ]] || {
  echo "Refusing restore without explicit confirmation" >&2; exit 2;
}

for cmd in age tar pg_restore psql python3; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing required tool: $cmd" >&2; exit 1; }
done

# Fail closed for obvious local production DB names unless separately overridden.
case "$VPNX3_RESTORE_DATABASE_URL" in
  *"/vpnx3?"*|*"/vpnx3") 
    [[ "${VPNX3_RESTORE_ALLOW_PRIMARY:-NO}" == "YES" ]] || {
      echo "Refusing restore into database named vpnx3. Use a dedicated drill DB." >&2; exit 2;
    };;
esac

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
age -d -i "$VPNX3_BACKUP_AGE_IDENTITY" "$VPNX3_BACKUP_FILE" | tar -C "$work" -xf -

pg_restore   --dbname="$VPNX3_RESTORE_DATABASE_URL"   --clean   --if-exists   --no-owner   --no-privileges   --exit-on-error   "$work/database.dump"

psql "$VPNX3_RESTORE_DATABASE_URL" -v ON_ERROR_STOP=1 -Atc "
SELECT 'users='||count(*) FROM users;
SELECT 'devices='||count(*) FROM devices;
SELECT 'nodes='||count(*) FROM nodes;
SELECT 'subscriptions='||count(*) FROM subscriptions;
SELECT 'payments='||count(*) FROM payments;
SELECT 'config_manifests='||count(*) FROM config_manifests;
SELECT 'releases='||count(*) FROM releases;
"

# Structural checks that catch many incomplete/corrupt restores.
psql "$VPNX3_RESTORE_DATABASE_URL" -v ON_ERROR_STOP=1 -Atc "
SELECT CASE WHEN EXISTS(
  SELECT 1 FROM information_schema.tables
  WHERE table_schema='public' AND table_name='audit_log'
) THEN 'audit_log=ok' ELSE CAST(1/0 AS text) END;
SELECT CASE WHEN EXISTS(
  SELECT 1 FROM information_schema.columns
  WHERE table_schema='public' AND table_name='devices' AND column_name='identity_public_key'
) THEN 'device_identity=ok' ELSE CAST(1/0 AS text) END;
"

echo "[OK] restore drill completed"
