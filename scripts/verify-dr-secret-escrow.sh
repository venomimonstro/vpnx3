#!/usr/bin/env bash
set -euo pipefail
umask 077

: "${VPNX3_DR_ESCROW_FILE:?VPNX3_DR_ESCROW_FILE is required}"
: "${VPNX3_DR_ESCROW_AGE_IDENTITY:?VPNX3_DR_ESCROW_AGE_IDENTITY is required}"

for cmd in age tar sha256sum python3; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing required tool: $cmd" >&2; exit 1; }
done
[[ -f "$VPNX3_DR_ESCROW_FILE" ]] || { echo "Escrow file missing" >&2; exit 1; }
[[ -f "$VPNX3_DR_ESCROW_AGE_IDENTITY" ]] || { echo "Escrow age identity missing" >&2; exit 1; }

if [[ -f "$VPNX3_DR_ESCROW_FILE.sha256" ]]; then
  (cd "$(dirname "$VPNX3_DR_ESCROW_FILE")" && sha256sum -c "$(basename "$VPNX3_DR_ESCROW_FILE").sha256")
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
age -d -i "$VPNX3_DR_ESCROW_AGE_IDENTITY" "$VPNX3_DR_ESCROW_FILE" | tar -C "$work" -xf -
[[ -f "$work/manifest.json" && -d "$work/files" ]] || { echo "Escrow bundle incomplete" >&2; exit 1; }

python3 - "$work" <<'PY'
import hashlib,json,pathlib,sys
root=pathlib.Path(sys.argv[1])
m=json.loads((root/"manifest.json").read_text("utf-8"))
if m.get("schema_version")!=1: raise SystemExit("unsupported escrow schema")
if m.get("offline_root_private_key_included") is not False:
    raise SystemExit("invalid escrow policy marker")
seen=set()
for item in m.get("items",[]):
    label=item.get("label","")
    if not label or label in seen: raise SystemExit("invalid/duplicate escrow label")
    seen.add(label)
    p=root/"files"/label
    if not p.is_file(): raise SystemExit(f"missing item {label}")
    h=hashlib.sha256(p.read_bytes()).hexdigest()
    if h!=item.get("sha256"): raise SystemExit(f"sha256 mismatch for {label}")
    if p.stat().st_size!=item.get("bytes"): raise SystemExit(f"size mismatch for {label}")
print(f"[OK] DR secret escrow verified: {len(seen)} item(s)")
PY
