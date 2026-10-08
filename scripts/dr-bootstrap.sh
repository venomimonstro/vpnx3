#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${VPNX3_BACKUP_FILE:?VPNX3_BACKUP_FILE is required}"
: "${VPNX3_BACKUP_AGE_IDENTITY:?VPNX3_BACKUP_AGE_IDENTITY is required}"
: "${VPNX3_RESTORE_DATABASE_URL:?VPNX3_RESTORE_DATABASE_URL is required}"
: "${VPNX3_DR_CONFIRM:?Set VPNX3_DR_CONFIRM=DR_ISOLATED_ENVIRONMENT}"

[[ "$VPNX3_DR_CONFIRM" == "DR_ISOLATED_ENVIRONMENT" ]] || {
  echo "Refusing DR bootstrap without explicit isolated-environment confirmation" >&2; exit 2;
}

MIGRATE_BINARY="${VPNX3_MIGRATE_BINARY:-/usr/local/bin/vpnx3-migrate}"
MIGRATIONS_DIR="${VPNX3_MIGRATIONS_DIR:-/migrations}"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

for cmd in psql age pg_restore; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing required tool: $cmd" >&2; exit 1; }
done
[[ -x "$MIGRATE_BINARY" ]] || { echo "Migration binary missing: $MIGRATE_BINARY" >&2; exit 1; }
[[ -d "$MIGRATIONS_DIR" ]] || { echo "Migrations directory missing: $MIGRATIONS_DIR" >&2; exit 1; }

case "$VPNX3_RESTORE_DATABASE_URL" in
  *"/vpnx3?"*|*"/vpnx3")
    [[ "${VPNX3_DR_ALLOW_PRIMARY_NAME:-NO}" == "YES" ]] || {
      echo "Refusing database named vpnx3 for DR bootstrap. Use an isolated database." >&2; exit 2;
    };;
esac

echo "==> 1/5 Verify encrypted backup"
VPNX3_BACKUP_FILE="$VPNX3_BACKUP_FILE" \
VPNX3_BACKUP_AGE_IDENTITY="$VPNX3_BACKUP_AGE_IDENTITY" \
  bash "$SCRIPT_DIR/verify-backup.sh"

echo "==> 2/5 Restore database"
VPNX3_BACKUP_FILE="$VPNX3_BACKUP_FILE" \
VPNX3_BACKUP_AGE_IDENTITY="$VPNX3_BACKUP_AGE_IDENTITY" \
VPNX3_RESTORE_DATABASE_URL="$VPNX3_RESTORE_DATABASE_URL" \
VPNX3_RESTORE_CONFIRM=RESTORE_ISOLATED_DATABASE \
  bash "$SCRIPT_DIR/restore-drill.sh"

echo "==> 3/5 Apply current migrations"
VPNX3_DATABASE_URL="$VPNX3_RESTORE_DATABASE_URL" \
VPNX3_MIGRATIONS_DIR="$MIGRATIONS_DIR" \
  "$MIGRATE_BINARY"

echo "==> 4/5 Verify schema and audit integrity"
psql "$VPNX3_RESTORE_DATABASE_URL" -v ON_ERROR_STOP=1 -At <<'SQL'
SELECT CASE WHEN verify_audit_chain() THEN 'audit_chain=ok' ELSE CAST(1/0 AS text) END;
SELECT 'schema_version='||COALESCE(max(version),0) FROM schema_migrations;
SELECT CASE WHEN EXISTS(
  SELECT 1 FROM information_schema.tables
  WHERE table_schema='public' AND table_name='device_revocation_snapshots'
) THEN 'revocations=ok' ELSE CAST(1/0 AS text) END;
SELECT CASE WHEN EXISTS(
  SELECT 1 FROM information_schema.tables
  WHERE table_schema='public' AND table_name='control_plane_leadership'
) THEN 'ha_state=ok' ELSE CAST(1/0 AS text) END;
SELECT CASE WHEN EXISTS(
  SELECT 1 FROM information_schema.tables
  WHERE table_schema='public' AND table_name='client_lease_rate_buckets'
) THEN 'lease_rate_limit=ok' ELSE CAST(1/0 AS text) END;
SQL

echo "==> 5/5 Business-state sanity checks"
psql "$VPNX3_RESTORE_DATABASE_URL" -v ON_ERROR_STOP=1 -At <<'SQL'
SELECT 'users='||count(*) FROM users;
SELECT 'active_devices='||count(*) FROM devices WHERE status='active';
SELECT 'active_subscriptions='||count(*) FROM subscriptions WHERE status IN ('active','grace');
SELECT 'succeeded_payments='||count(*) FROM payments WHERE status IN ('succeeded','refunded');
SELECT 'active_nodes='||count(*) FROM nodes WHERE status='active';
SELECT 'published_releases='||count(*) FROM releases WHERE status='published';
SQL

echo
echo "[OK] isolated DR bootstrap database is structurally ready."
echo "Do NOT expose it publicly yet."
echo "Next: generate/restore operational secrets, install root-signed trust bundle, start Control Plane privately and pass launch readiness."
