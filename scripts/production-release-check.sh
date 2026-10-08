#!/usr/bin/env bash
set -euo pipefail

: "${VPNX3_CHECK_CONTROL_URL:?VPNX3_CHECK_CONTROL_URL is required}"
: "${VPNX3_CHECK_ADMIN_EMAIL:?VPNX3_CHECK_ADMIN_EMAIL is required}"
: "${VPNX3_CHECK_ADMIN_PASSWORD:?VPNX3_CHECK_ADMIN_PASSWORD is required}"

RELEASE_ID="${VPNX3_CHECK_RELEASE_ID:-}"

for cmd in curl python3; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing required tool: $cmd" >&2; exit 1; }
done

[[ "$VPNX3_CHECK_CONTROL_URL" == https://* ]] || {
  echo "Control URL must use https" >&2; exit 2;
}

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

login_payload="$(python3 - "$VPNX3_CHECK_ADMIN_EMAIL" "$VPNX3_CHECK_ADMIN_PASSWORD" <<'PY'
import json,sys
print(json.dumps({"email":sys.argv[1],"password":sys.argv[2]}))
PY
)"

curl --fail --silent --show-error   --connect-timeout 5 --max-time 15   -H 'Content-Type: application/json'   --data "$login_payload"   "$VPNX3_CHECK_CONTROL_URL/api/v1/admin/login" >"$tmp"

token="$(python3 - "$tmp" <<'PY'
import json,sys
j=json.load(open(sys.argv[1]))
t=j.get("token","")
if not t: raise SystemExit("login response has no token")
print(t)
PY
)"

check_json(){
  local url="$1"
  curl --fail --silent --show-error     --connect-timeout 5 --max-time 20     -H "Authorization: Bearer $token"     "$url"
}

readiness="$(check_json "$VPNX3_CHECK_CONTROL_URL/api/v1/admin/readiness")"
printf '%s\n' "$readiness" | python3 -c '
import json,sys
j=json.load(sys.stdin)
print("Launch readiness:",j.get("status"))
for c in j.get("checks",[]):
    print(f" - {c.get("status")}: {c.get("title")} — {c.get("detail")}")
if j.get("status")=="failed": raise SystemExit(10)
'

if [[ -n "$RELEASE_ID" ]]; then
  gate="$(check_json "$VPNX3_CHECK_CONTROL_URL/api/v1/admin/releases/$RELEASE_ID/gate")"
  printf '%s\n' "$gate" | python3 -c '
import json,sys
j=json.load(sys.stdin)
print("Release gate:",j.get("status"),"enforced=",j.get("enforced"))
for x in j.get("blockers",[]): print(" BLOCKER:",x)
for x in j.get("warnings",[]): print(" WARNING:",x)
if j.get("enforced") and j.get("blockers"): raise SystemExit(11)
'
fi

curl --silent --show-error --max-time 10   -X POST -H "Authorization: Bearer $token"   "$VPNX3_CHECK_CONTROL_URL/api/v1/admin/logout" >/dev/null || true

echo "[OK] production release check passed"
