#!/usr/bin/env bash
set -euo pipefail

CONTROL_URL="${VPNX3_DRILL_CONTROL_URL:-}"
ADMIN_EMAIL="${VPNX3_DRILL_ADMIN_EMAIL:-}"
ADMIN_PASSWORD="${VPNX3_DRILL_ADMIN_PASSWORD:-}"
NODE_ID="${VPNX3_DRILL_NODE_ID:-}"
ALLOW_PRODUCTION="${VPNX3_DRILL_ALLOW_PRODUCTION:-false}"
TIMEOUT="${VPNX3_DRILL_TIMEOUT_SECONDS:-10}"
POLL_SECONDS="${VPNX3_DRILL_POLL_SECONDS:-2}"
MAX_WAIT_SECONDS="${VPNX3_DRILL_MAX_WAIT_SECONDS:-40}"

[[ "$CONTROL_URL" == https://* ]] || { echo "[FAIL] control URL must use https" >&2; exit 2; }
[[ -n "$ADMIN_EMAIL" && -n "$ADMIN_PASSWORD" && -n "$NODE_ID" ]] || {
  echo "[FAIL] admin credentials and VPNX3_DRILL_NODE_ID are required" >&2; exit 2;
}
for cmd in curl python3 mktemp; do command -v "$cmd" >/dev/null || { echo "[FAIL] missing $cmd" >&2; exit 2; }; done

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
TOKEN=""
DRAINED=false

request(){
  local method="$1" path="$2" outfile="$3" auth="${4:-}" data="${5:-}"
  local args=(--silent --show-error --location --max-time "$TIMEOUT"
    --request "$method" --output "$outfile" --write-out "%{http_code}"
    --header "Accept: application/json")
  [[ -n "$auth" ]] && args+=(--header "Authorization: Bearer $auth")
  [[ -n "$data" ]] && args+=(--header "Content-Type: application/json" --data-binary "$data")
  curl "${args[@]}" "$CONTROL_URL$path"
}

json_value(){
  local file="$1" expr="$2"
  python3 - "$file" "$expr" <<'PY'
import json,sys
with open(sys.argv[1],encoding="utf-8") as f:data=json.load(f)
cur=data
for part in sys.argv[2].split("."):
    if not part: continue
    cur=cur.get(part) if isinstance(cur,dict) else None
    if cur is None: break
if isinstance(cur,bool): print("true" if cur else "false")
elif cur is None: print("")
else: print(cur)
PY
}

manifest_snapshot(){
  local file="$1"
  python3 - "$file" "$NODE_ID" <<'PY'
import base64,json,sys
with open(sys.argv[1],encoding="utf-8") as f: env=json.load(f)
raw=base64.urlsafe_b64decode(env["payload"]+"="*((4-len(env["payload"])%4)%4))
p=json.loads(raw)
node=sys.argv[2]
present=any(x.get("id")==node for x in p.get("workers",[])+p.get("ingresses",[]))
print("%s %s" % (p.get("version",0),"true" if present else "false"))
PY
}

restore(){
  if [[ "$DRAINED" == "true" && -n "$TOKEN" ]]; then
    echo "[INFO] emergency restore: returning node to active"
    local body="$tmpdir/restore.json"
    request POST "/api/v1/nodes/$NODE_ID/publish" "$body" "$TOKEN" '{"reason":"automatic restore after failover drill"}' >/dev/null || true
  fi
}
trap 'restore; rm -rf "$tmpdir"' EXIT

meta="$tmpdir/meta.json"
status="$(request GET "/api/v1/meta" "$meta" || true)"
[[ "$status" == "200" ]] || { echo "[FAIL] metadata HTTP $status"; exit 1; }
environment="$(json_value "$meta" "environment")"
if [[ "$environment" == "production" && "$ALLOW_PRODUCTION" != "YES" ]]; then
  echo "[FAIL] refusing production failover drill. Set VPNX3_DRILL_ALLOW_PRODUCTION=YES explicitly." >&2
  exit 3
fi
echo "[ OK ] environment=$environment"

login="$tmpdir/login.json"
payload="$(python3 - "$ADMIN_EMAIL" "$ADMIN_PASSWORD" <<'PY'
import json,sys
print(json.dumps({"email":sys.argv[1],"password":sys.argv[2]}))
PY
)"
status="$(request POST "/api/v1/admin/login" "$login" "" "$payload" || true)"
[[ "$status" == "200" ]] || { echo "[FAIL] admin login HTTP $status"; exit 1; }
TOKEN="$(json_value "$login" "token")"
[[ -n "$TOKEN" ]] || { echo "[FAIL] no bearer token"; exit 1; }

node="$tmpdir/node.json"
status="$(request GET "/api/v1/nodes/$NODE_ID" "$node" "$TOKEN" || true)"
[[ "$status" == "200" ]] || { echo "[FAIL] node lookup HTTP $status"; exit 1; }
role="$(json_value "$node" "role")"
node_status="$(json_value "$node" "status")"
[[ "$node_status" == "active" ]] || { echo "[FAIL] node must be active, got $node_status"; exit 1; }
[[ "$role" == "worker" || "$role" == "ingress" ]] || { echo "[FAIL] drill supports worker/ingress only"; exit 1; }

ready="$tmpdir/readiness.json"
status="$(request GET "/api/v1/admin/readiness" "$ready" "$TOKEN" || true)"
[[ "$status" == "200" ]] || { echo "[FAIL] readiness HTTP $status"; exit 1; }
python3 - "$ready" "$role" <<'PY'
import json,sys
with open(sys.argv[1],encoding="utf-8") as f:d=json.load(f)
s=d.get("signals",{})
role=sys.argv[2]
count=s.get("routable_workers",0) if role=="worker" else s.get("active_ingresses",0)
if count < 2:
    raise SystemExit("need at least 2 routable nodes for a safe failover drill")
print("[ OK ] redundancy before drill:",count)
PY

before="$tmpdir/config-before.json"
status="$(request GET "/api/v1/config/latest" "$before" || true)"
[[ "$status" == "200" ]] || { echo "[FAIL] latest config HTTP $status"; exit 1; }
read -r version_before present_before <<<"$(manifest_snapshot "$before")"
[[ "$present_before" == "true" ]] || { echo "[FAIL] selected active node is absent from manifest"; exit 1; }
echo "[ OK ] manifest v$version_before contains selected node"

drain="$tmpdir/drain.json"
status="$(request POST "/api/v1/nodes/$NODE_ID/drain" "$drain" "$TOKEN" '{"reason":"controlled failover drill"}' || true)"
[[ "$status" == "200" ]] || { echo "[FAIL] drain HTTP $status"; exit 1; }
DRAINED=true
echo "[ OK ] node moved to draining"

wait_manifest(){
  local min_version="$1" expected_presence="$2" label="$3"
  local elapsed=0
  while (( elapsed < MAX_WAIT_SECONDS )); do
    local file="$tmpdir/poll.json" status version present
    status="$(request GET "/api/v1/config/latest" "$file" || true)"
    if [[ "$status" == "200" ]]; then
      read -r version present <<<"$(manifest_snapshot "$file")"
      if (( version > min_version )) && [[ "$present" == "$expected_presence" ]]; then
        echo "$version"
        return 0
      fi
    fi
    sleep "$POLL_SECONDS"
    elapsed=$((elapsed+POLL_SECONDS))
  done
  echo "[FAIL] $label did not converge within ${MAX_WAIT_SECONDS}s" >&2
  return 1
}

version_drained="$(wait_manifest "$version_before" "false" "drain manifest")"
echo "[ OK ] manifest v$version_drained removed drained node"

publish="$tmpdir/publish.json"
status="$(request POST "/api/v1/nodes/$NODE_ID/publish" "$publish" "$TOKEN" '{"reason":"restore after controlled failover drill"}' || true)"
[[ "$status" == "200" ]] || { echo "[FAIL] restore publish HTTP $status"; exit 1; }
DRAINED=false
echo "[ OK ] node returned to active"

version_restored="$(wait_manifest "$version_drained" "true" "restore manifest")"
echo "[ OK ] manifest v$version_restored restored node"

final_ready="$tmpdir/final-readiness.json"
status="$(request GET "/api/v1/admin/readiness" "$final_ready" "$TOKEN" || true)"
[[ "$status" == "200" ]] || { echo "[FAIL] final readiness HTTP $status"; exit 1; }
overall="$(json_value "$final_ready" "status")"
[[ "$overall" != "failed" ]] || { echo "[FAIL] readiness failed after restore"; exit 1; }
echo "[ OK ] final readiness=$overall"

request POST "/api/v1/admin/logout" "$tmpdir/logout.json" "$TOKEN" >/dev/null || true
TOKEN=""
echo
echo "Failover drill completed successfully."
