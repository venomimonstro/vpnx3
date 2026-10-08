#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

FAST=false
if [[ "${1:-}" == "--fast" ]]; then FAST=true; shift; fi
[[ $# -eq 0 ]] || { echo "Usage: scripts/predeploy-check.sh [--fast]" >&2; exit 2; }

for cmd in go bash python3 find sort awk grep; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "[FAIL] missing required tool: $cmd" >&2; exit 2; }
done

echo "==> 1/7 Git hygiene"
if find .github/workflows -type f 2>/dev/null | grep -q .; then
  echo "[FAIL] GitHub Actions/workflows are not allowed for this project." >&2
  exit 1
fi
if command -v git >/dev/null 2>&1; then
  git diff --check
fi
echo "[ OK ] repository hygiene"

echo "==> 2/7 Migration numbering"
python3 - <<'PY'
from pathlib import Path
import re,sys
seen={}
for p in sorted(Path("migrations").glob("*.sql")):
    m=re.match(r"^(\d{6})_",p.name)
    if not m:
        print(f"[FAIL] migration without 6-digit prefix: {p}",file=sys.stderr);sys.exit(1)
    prefix=m.group(1)
    if prefix in seen:
        print(f"[FAIL] duplicate migration prefix {prefix}: {seen[prefix]} and {p}",file=sys.stderr);sys.exit(1)
    seen[prefix]=p
print(f"[ OK ] {len(seen)} migration prefixes are unique")
PY

echo "==> 3/7 Shell syntax"
while IFS= read -r -d '' file; do
  bash -n "$file"
done < <(find scripts -type f -name '*.sh' -print0)
echo "[ OK ] shell syntax"

echo "==> 4/7 Python syntax"
while IFS= read -r -d '' file; do
  python3 -m py_compile "$file"
done < <(find scripts -type f -name '*.py' -print0)
find scripts -type d -name '__pycache__' -prune -exec rm -rf {} +
echo "[ OK ] python syntax"

echo "==> 5/7 JavaScript syntax"
if command -v node >/dev/null 2>&1; then
  while IFS= read -r -d '' file; do
    node --check "$file" >/dev/null
  done < <(find internal/adminui/static apps/browser-extension -type f -name '*.js' -print0 2>/dev/null)
  echo "[ OK ] JavaScript syntax"
else
  echo "[WARN] node is unavailable; JavaScript syntax check skipped"
fi

echo "==> 6/7 Go formatting/vet"
unformatted="$(gofmt -l cmd internal network 2>/dev/null || true)"
if [[ -n "$unformatted" ]]; then
  echo "[FAIL] gofmt required:" >&2
  printf '%s\n' "$unformatted" >&2
  exit 1
fi
go vet ./...
echo "[ OK ] gofmt + go vet"

echo "==> 7/7 Go tests"
if [[ "$FAST" == "true" ]]; then
  go test ./internal/resilience ./internal/httpapi
else
  go test ./...
fi
echo "[ OK ] Go tests"

echo
echo "PREDEPLOY CHECK: OK"
