#!/usr/bin/env bash
set -euo pipefail
umask 077

RECIPIENT=""
OUT=""
declare -a ITEMS=()

usage(){
cat <<'EOF'
Create VPNX3 encrypted DR secret escrow.

Required:
  --recipient age1...
  --out /secure/offline/vpnx3-dr-secrets-YYYYMMDD.tar.age
  --file label=/absolute/path

Repeat --file for each explicitly approved recovery item.

Important:
- Do NOT include the offline trust-root private seed in this escrow.
- Use a different age recipient from ordinary data backups where possible.
- Source files must be regular files, not symlinks.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --recipient) RECIPIENT="${2:-}"; shift 2;;
    --out) OUT="${2:-}"; shift 2;;
    --file) ITEMS+=("${2:-}"); shift 2;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2;;
  esac
done

[[ "$RECIPIENT" == age1* ]] || { echo "age recipient is required" >&2; exit 2; }
[[ "$OUT" == /* ]] || { echo "--out must be an absolute path" >&2; exit 2; }
(( ${#ITEMS[@]} > 0 )) || { echo "At least one --file is required" >&2; exit 2; }
for cmd in age tar sha256sum python3 realpath stat; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing required tool: $cmd" >&2; exit 1; }
done

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/files"
manifest="$work/manifest.json"
rows="$work/rows.tsv"
: >"$rows"

declare -A seen=()
for item in "${ITEMS[@]}"; do
  label="${item%%=*}"
  path="${item#*=}"
  [[ -n "$label" && "$path" != "$item" ]] || { echo "Invalid --file $item" >&2; exit 2; }
  [[ "$label" =~ ^[A-Za-z0-9._-]{1,80}$ ]] || { echo "Invalid label: $label" >&2; exit 2; }
  [[ -z "${seen[$label]:-}" ]] || { echo "Duplicate label: $label" >&2; exit 2; }
  seen[$label]=1
  [[ "$path" == /* ]] || { echo "Secret path must be absolute: $path" >&2; exit 2; }
  [[ -f "$path" && ! -L "$path" ]] || { echo "Secret must be a regular non-symlink file: $path" >&2; exit 2; }
  resolved="$(realpath -e "$path")"
  [[ -f "$resolved" && ! -L "$resolved" ]] || { echo "Resolved secret path invalid: $path" >&2; exit 2; }
  mode="$(stat -c "%a" "$resolved")"
  case "$mode" in
    400|600) ;;
    *) echo "Secret file $path must have mode 0400 or 0600" >&2; exit 2;;
  esac
  dest="$work/files/$label"
  install -m 0600 "$resolved" "$dest"
  sha="$(sha256sum "$dest" | awk '{print $1}')"
  bytes="$(wc -c <"$dest" | tr -d " ")"
  printf "%s\t%s\t%s\n" "$label" "$sha" "$bytes" >>"$rows"
done

python3 - "$manifest" "$rows" <<'PY'
import json,sys,time,pathlib
manifest,rows=sys.argv[1:]
items=[]
for line in pathlib.Path(rows).read_text("utf-8").splitlines():
    label,sha,size=line.split("\t")
    items.append({"label":label,"sha256":sha,"bytes":int(size)})
payload={
  "schema_version":1,
  "created_at":time.strftime("%Y-%m-%dT%H:%M:%SZ",time.gmtime()),
  "items":items,
  "offline_root_private_key_included":False,
}
pathlib.Path(manifest).write_text(json.dumps(payload,ensure_ascii=False,sort_keys=True,indent=2)+"\n","utf-8")
PY

mkdir -p "$(dirname "$OUT")"
tmp="$OUT.partial"
rm -f "$tmp"
tar -C "$work" -cf - manifest.json files | age -r "$RECIPIENT" -o "$tmp"
chmod 600 "$tmp"
mv "$tmp" "$OUT"
sha256sum "$OUT" >"$OUT.sha256"
chmod 600 "$OUT.sha256"
echo "$OUT"
