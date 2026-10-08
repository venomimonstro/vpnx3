#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${VPNX3_BACKUP_DATABASE_URL:?VPNX3_BACKUP_DATABASE_URL is required}"
: "${VPNX3_BACKUP_DIR:?VPNX3_BACKUP_DIR is required}"
: "${VPNX3_BACKUP_AGE_RECIPIENT:?VPNX3_BACKUP_AGE_RECIPIENT is required}"

RETENTION_DAYS="${VPNX3_BACKUP_RETENTION_DAYS:-35}"
ARTIFACT_DIR="${VPNX3_BACKUP_ARTIFACT_DIR:-}"
INCLUDE_ARTIFACTS="${VPNX3_BACKUP_INCLUDE_ARTIFACTS:-no}"

for cmd in pg_dump pg_restore age sha256sum tar python3 date find; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing required tool: $cmd" >&2; exit 1; }
done

[[ "$RETENTION_DAYS" =~ ^[0-9]+$ ]] && (( RETENTION_DAYS >= 7 && RETENTION_DAYS <= 3650 )) || {
  echo "VPNX3_BACKUP_RETENTION_DAYS must be 7..3650" >&2; exit 2;
}
[[ "$VPNX3_BACKUP_DIR" == /* ]] || { echo "VPNX3_BACKUP_DIR must be absolute" >&2; exit 2; }

mkdir -p "$VPNX3_BACKUP_DIR"
chmod 700 "$VPNX3_BACKUP_DIR"

ts="$(date -u +%Y%m%dT%H%M%SZ)"
host="$(hostname 2>/dev/null || echo unknown)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

db="$work/database.dump"
manifest="$work/manifest.json"
bundle="$VPNX3_BACKUP_DIR/vpnx3-$ts.tar.age"
tmp_bundle="$bundle.partial"

pg_dump   --dbname="$VPNX3_BACKUP_DATABASE_URL"   --format=custom   --compress=9   --no-owner   --no-privileges   --file="$db"

# Prove that pg_restore can at least parse the archive before encrypting it.
pg_restore --list "$db" >/dev/null

db_sha="$(sha256sum "$db" | awk '{print $1}')"
db_bytes="$(wc -c <"$db" | tr -d ' ')"

artifacts_included=false
artifact_sha=""
artifact_bytes=0
if [[ "$INCLUDE_ARTIFACTS" == "yes" ]]; then
  [[ -n "$ARTIFACT_DIR" && -d "$ARTIFACT_DIR" ]] || {
    echo "Artifact backup requested but VPNX3_BACKUP_ARTIFACT_DIR is unavailable" >&2; exit 1;
  }
  tar --numeric-owner --owner=0 --group=0 -C "$ARTIFACT_DIR" -cf "$work/artifacts.tar" .
  artifact_sha="$(sha256sum "$work/artifacts.tar" | awk '{print $1}')"
  artifact_bytes="$(wc -c <"$work/artifacts.tar" | tr -d ' ')"
  artifacts_included=true
fi

python3 - "$manifest" "$ts" "$host" "$db_sha" "$db_bytes" "$artifacts_included" "$artifact_sha" "$artifact_bytes" <<'PY'
import json,sys
path,ts,host,dbsha,dbbytes,artifacts,asha,abytes=sys.argv[1:]
payload={
  "schema_version":1,
  "created_at":ts,
  "source_host":host,
  "database":{"format":"postgres-custom","sha256":dbsha,"bytes":int(dbbytes)},
  "artifacts":{
    "included":artifacts=="true",
    "sha256":asha or None,
    "bytes":int(abytes),
  },
  "contains_secrets":False,
  "restore_requires_external_secret_escrow":True,
}
open(path,"w",encoding="utf-8").write(json.dumps(payload,ensure_ascii=False,sort_keys=True,indent=2)+"\n")
PY

tar -C "$work" -cf - database.dump manifest.json $([[ "$artifacts_included" == true ]] && printf '%s' artifacts.tar)   | age -r "$VPNX3_BACKUP_AGE_RECIPIENT" -o "$tmp_bundle"

chmod 600 "$tmp_bundle"
mv "$tmp_bundle" "$bundle"
sha256sum "$bundle" >"$bundle.sha256"
chmod 600 "$bundle.sha256"

# Never delete recent backups. Only completed .tar.age bundles older than retention.
find "$VPNX3_BACKUP_DIR" -type f \( -name 'vpnx3-*.tar.age' -o -name 'vpnx3-*.tar.age.sha256' \)   -mtime +"$RETENTION_DAYS" -delete

echo "$bundle"
