#!/usr/bin/env bash
set -euo pipefail

CONTROL_URL="${VPNX3_SMOKE_CONTROL_URL:-}"
ADMIN_EMAIL="${VPNX3_SMOKE_ADMIN_EMAIL:-}"
ADMIN_PASSWORD="${VPNX3_SMOKE_ADMIN_PASSWORD:-}"
FAIL_ON_WARNING="${VPNX3_SMOKE_FAIL_ON_WARNING:-false}"
TIMEOUT="${VPNX3_SMOKE_TIMEOUT_SECONDS:-10}"

usage(){
  cat <<'EOF'
VPNX3 production smoke suite.

Required environment:
  VPNX3_SMOKE_CONTROL_URL=https://control.example
  VPNX3_SMOKE_ADMIN_EMAIL=owner@example.com
  VPNX3_SMOKE_ADMIN_PASSWORD='...'

Optional:
  VPNX3_SMOKE_FAIL_ON_WARNING=true
  VPNX3_SMOKE_TIMEOUT_SECONDS=10

The script is manual and does not use CI/Actions.
It never prints the admin password or bearer token.
EOF
}

[[ "${1:-}" == "--help" ]] && { usage; exit 0; }

[[ "$CONTROL_URL" == https://* ]] || { echo "[FAIL] VPNX3_SMOKE_CONTROL_URL must use https" >&2; exit 2; }
[[ -n "$ADMIN_EMAIL" ]] || { echo "[FAIL] VPNX3_SMOKE_ADMIN_EMAIL is required" >&2; exit 2; }
[[ -n "$ADMIN_PASSWORD" ]] || { echo "[FAIL] VPNX3_SMOKE_ADMIN_PASSWORD is required" >&2; exit 2; }
[[ "$FAIL_ON_WARNING" == "true" || "$FAIL_ON_WARNING" == "false" ]] || {
  echo "[FAIL] VPNX3_SMOKE_FAIL_ON_WARNING must be true or false" >&2; exit 2;
}
[[ "$TIMEOUT" =~ ^[0-9]+$ ]] && (( TIMEOUT >= 2 && TIMEOUT <= 60 )) || {
  echo "[FAIL] VPNX3_SMOKE_TIMEOUT_SECONDS must be 2..60" >&2; exit 2;
}

for cmd in curl python3 mktemp; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "[FAIL] missing command: $cmd" >&2; exit 2; }
done

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

PASS=0
WARN=0
FAIL=0

request(){
  local method="$1" path="$2" outfile="$3" auth="${4:-}" data="${5:-}"
  local args=(--silent --show-error --location --max-time "$TIMEOUT"
    --request "$method" --output "$outfile" --write-out "%{http_code}"
    --header "Accept: application/json")
  [[ -n "$auth" ]] && args+=(--header "Authorization: Bearer $auth")
  if [[ -n "$data" ]]; then
    args+=(--header "Content-Type: application/json" --data-binary "$data")
  fi
  curl "${args[@]}" "$CONTROL_URL$path"
}

check_public(){
  local name="$1" path="$2" expected="${3:-200}"
  local body="$tmpdir/public-${PASS}-${WARN}-${FAIL}.json" status
  if ! status="$(request GET "$path" "$body")"; then
    echo "[FAIL] $name: network error"
    FAIL=$((FAIL+1)); return
  fi
  if [[ "$status" == "$expected" ]]; then
    echo "[ OK ] $name"
    PASS=$((PASS+1))
  else
    echo "[FAIL] $name: HTTP $status"
    cat "$body" 2>/dev/null || true
    echo
    FAIL=$((FAIL+1))
  fi
}

check_public "liveness" "/health/live"
check_public "readiness" "/health/ready"
check_public "metadata" "/api/v1/meta"
check_public "config signing key" "/api/v1/config/signing-key"
check_public "latest signed config" "/api/v1/config/latest"
check_public "public plans" "/api/v1/plans"

login_payload="$(python3 - "$ADMIN_EMAIL" "$ADMIN_PASSWORD" <<'PY'
import json,sys
print(json.dumps({"email":sys.argv[1],"password":sys.argv[2]},ensure_ascii=False))
PY
)"
login_body="$tmpdir/login.json"
login_status="$(request POST "/api/v1/admin/login" "$login_body" "" "$login_payload" || true)"
if [[ "$login_status" != "200" ]]; then
  echo "[FAIL] admin login: HTTP $login_status"
  FAIL=$((FAIL+1))
  echo
  echo "Smoke summary: PASS=$PASS WARN=$WARN FAIL=$FAIL"
  exit 1
fi
TOKEN="$(python3 - "$login_body" <<'PY'
import json,sys
with open(sys.argv[1],encoding="utf-8") as f:
    data=json.load(f)
token=data.get("token","")
if not token:
    raise SystemExit(1)
print(token)
PY
)" || { echo "[FAIL] admin login response has no token"; exit 1; }
echo "[ OK ] admin login"
PASS=$((PASS+1))

me_body="$tmpdir/me.json"
me_status="$(request GET "/api/v1/admin/me" "$me_body" "$TOKEN" || true)"
if [[ "$me_status" == "200" ]]; then
  echo "[ OK ] authenticated admin API"
  PASS=$((PASS+1))
else
  echo "[FAIL] authenticated admin API: HTTP $me_status"
  FAIL=$((FAIL+1))
fi

ready_body="$tmpdir/readiness.json"
ready_status="$(request GET "/api/v1/admin/readiness" "$ready_body" "$TOKEN" || true)"
if [[ "$ready_status" != "200" ]]; then
  echo "[FAIL] launch readiness endpoint: HTTP $ready_status"
  FAIL=$((FAIL+1))
else
  overall="$(python3 - "$ready_body" <<'PY'
import json,sys
with open(sys.argv[1],encoding="utf-8") as f:
    data=json.load(f)
print(data.get("status","failed"))
for item in data.get("checks",[]):
    print("%s\t%s\t%s" % (
        item.get("status","failed"),
        item.get("title","unknown"),
        item.get("detail","")
    ),file=sys.stderr)
PY
  2>"$tmpdir/readiness-lines")"
  while IFS=$'\t' read -r status title detail; do
    case "$status" in
      ok)      printf '[ OK ] %s — %s\n' "$title" "$detail"; PASS=$((PASS+1));;
      warning) printf '[WARN] %s — %s\n' "$title" "$detail"; WARN=$((WARN+1));;
      *)       printf '[FAIL] %s — %s\n' "$title" "$detail"; FAIL=$((FAIL+1));;
    esac
  done <"$tmpdir/readiness-lines"
  if [[ "$overall" == "failed" ]]; then
    echo "[FAIL] commercial launch preflight"
  elif [[ "$overall" == "warning" ]]; then
    echo "[WARN] commercial launch preflight"
  else
    echo "[ OK ] commercial launch preflight"
  fi
fi

release_key_body="$tmpdir/release-key.json"
release_key_status="$(request GET "/api/v1/releases/signing-key" "$release_key_body" || true)"
case "$release_key_status" in
  200) echo "[ OK ] public release signing key"; PASS=$((PASS+1));;
  503) echo "[WARN] public release channel is disabled"; WARN=$((WARN+1));;
  *)   echo "[FAIL] release signing endpoint: HTTP $release_key_status"; FAIL=$((FAIL+1));;
esac

logout_body="$tmpdir/logout.json"
logout_status="$(request POST "/api/v1/admin/logout" "$logout_body" "$TOKEN" || true)"
if [[ "$logout_status" == "204" ]]; then
  echo "[ OK ] admin logout"
  PASS=$((PASS+1))
else
  echo "[WARN] admin logout returned HTTP $logout_status"
  WARN=$((WARN+1))
fi

echo
echo "Smoke summary: PASS=$PASS WARN=$WARN FAIL=$FAIL"

if (( FAIL > 0 )); then exit 1; fi
if [[ "$FAIL_ON_WARNING" == "true" ]] && (( WARN > 0 )); then exit 2; fi
exit 0
