#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

for cmd in go python3; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing required tool: $cmd" >&2; exit 1; }
done

python3 - <<'PY'
import pathlib,re,sys
root=pathlib.Path("migrations")
versions={}
errors=[]
for p in sorted(root.glob("*.sql")):
    m=re.match(r"^(\d+)_",p.name)
    if not m:
        errors.append(f"invalid migration filename: {p.name}")
        continue
    v=int(m.group(1))
    if v in versions:
        errors.append(f"duplicate migration version {v}: {versions[v]} and {p.name}")
    versions[v]=p.name
if errors:
    print("\n".join("FAIL: "+e for e in errors),file=sys.stderr)
    raise SystemExit(1)
print(f"[OK] {len(versions)} unique migration versions")
PY

go test ./internal/config ./internal/httpapi ./internal/buildworker ./internal/deviceauth ./internal/clientconfig ./internal/accesslease ./internal/artifactstorage

python3 scripts/validate-ios-source.py
python3 scripts/validate-browser-extension.py --browser chrome --dir apps/browser-extension/chrome
python3 scripts/validate-browser-extension.py --browser firefox --dir apps/browser-extension/firefox

# Reject obvious secret material accidentally committed to source directories.
if grep -R -n -E --exclude-dir=.git --exclude='*.md'   'BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|AKIA[0-9A-Z]{16}'   cmd internal apps scripts deploy migrations >/tmp/vpnx3-secret-scan.txt; then
  cat /tmp/vpnx3-secret-scan.txt >&2
  echo "FAIL: possible private key/credential material found" >&2
  exit 1
fi
rm -f /tmp/vpnx3-secret-scan.txt

echo "[OK] local quality gate passed"
