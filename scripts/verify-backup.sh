#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${VPNX3_BACKUP_FILE:?VPNX3_BACKUP_FILE is required}"
: "${VPNX3_BACKUP_AGE_IDENTITY:?VPNX3_BACKUP_AGE_IDENTITY is required}"

for cmd in age sha256sum tar pg_restore python3; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing required tool: $cmd" >&2; exit 1; }
done

[[ -f "$VPNX3_BACKUP_FILE" ]] || { echo "Backup file missing" >&2; exit 1; }
[[ -f "$VPNX3_BACKUP_AGE_IDENTITY" ]] || { echo "age identity missing" >&2; exit 1; }

if [[ -f "$VPNX3_BACKUP_FILE.sha256" ]]; then
  (cd "$(dirname "$VPNX3_BACKUP_FILE")" && sha256sum -c "$(basename "$VPNX3_BACKUP_FILE").sha256")
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
age -d -i "$VPNX3_BACKUP_AGE_IDENTITY" "$VPNX3_BACKUP_FILE" | tar -C "$work" -xf -

[[ -f "$work/database.dump" && -f "$work/manifest.json" ]] || {
  echo "Backup bundle is incomplete" >&2; exit 1;
}
pg_restore --list "$work/database.dump" >/dev/null

python3 - "$work" <<'PY'
import hashlib,json,pathlib,sys
root=pathlib.Path(sys.argv[1])
m=json.loads((root/"manifest.json").read_text("utf-8"))
if m.get("schema_version")!=1: raise SystemExit("unsupported backup schema")
def sha(path):
    h=hashlib.sha256()
    with open(path,"rb") as f:
        for chunk in iter(lambda:f.read(1024*1024),b""): h.update(chunk)
    return h.hexdigest()
db=root/"database.dump"
if sha(db)!=m["database"]["sha256"]: raise SystemExit("database checksum mismatch")
if db.stat().st_size!=m["database"]["bytes"]: raise SystemExit("database size mismatch")
if m["artifacts"]["included"]:
    p=root/"artifacts.tar"
    if not p.exists(): raise SystemExit("artifact archive missing")
    if sha(p)!=m["artifacts"]["sha256"]: raise SystemExit("artifact checksum mismatch")
print("[OK] encrypted backup bundle verified")
PY
