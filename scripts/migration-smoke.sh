#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE="${VPNX3_MIGRATION_SMOKE_IMAGE:-postgres:16-alpine}"
NAME="vpnx3-migration-smoke-$$"
PORT="${VPNX3_MIGRATION_SMOKE_PORT:-55439}"

command -v docker >/dev/null 2>&1 || { echo "docker is required" >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "go is required" >&2; exit 1; }

cleanup(){ docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --rm --name "$NAME"   -e POSTGRES_DB=vpnx3_smoke   -e POSTGRES_USER=vpnx3   -e POSTGRES_PASSWORD=smoke_only   -p "127.0.0.1:$PORT:5432" "$IMAGE" >/dev/null

for _ in $(seq 1 60); do
  if docker exec "$NAME" pg_isready -U vpnx3 -d vpnx3_smoke >/dev/null 2>&1; then break; fi
  sleep 1
done
docker exec "$NAME" pg_isready -U vpnx3 -d vpnx3_smoke >/dev/null

export VPNX3_ENV=development
export VPNX3_DATABASE_URL="postgres://vpnx3:smoke_only@127.0.0.1:$PORT/vpnx3_smoke?sslmode=disable"
# Deterministic distinct test-only 32-byte seeds.
export VPNX3_CONFIG_SIGNING_KEY="AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE"
export VPNX3_ACCESS_SIGNING_KEY="AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI"
export VPNX3_RELEASE_SIGNING_KEY="AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM"
export VPNX3_MIGRATIONS_DIR="$ROOT/migrations"

go run ./cmd/migrate

psql_exec(){
  docker exec -i "$NAME" psql -v ON_ERROR_STOP=1 -U vpnx3 -d vpnx3_smoke "$@"
}

applied="$(psql_exec -Atc 'SELECT count(*) FROM schema_migrations')"
files="$(find "$ROOT/migrations" -maxdepth 1 -type f -name '*.sql' | wc -l | tr -d ' ')"
[[ "$applied" == "$files" ]] || { echo "migration count mismatch: db=$applied files=$files" >&2; exit 1; }

psql_exec -Atc "SELECT verify_audit_chain()" | grep -qx t
psql_exec -c "INSERT INTO audit_log(actor_type,action,resource_type,result) VALUES('smoke','test','migration','success')" >/dev/null
psql_exec -Atc "SELECT verify_audit_chain()" | grep -qx t

if psql_exec -c "UPDATE audit_log SET result='tampered' WHERE action='test'" >/dev/null 2>&1; then
  echo "audit append-only trigger failed" >&2
  exit 1
fi

psql_exec -Atc "SELECT to_regclass('public.client_registration_rate') IS NOT NULL" | grep -qx t
psql_exec -Atc "SELECT to_regclass('public.device_pairing_codes') IS NOT NULL" | grep -qx t
psql_exec -Atc "SELECT to_regclass('public.release_publication_attestations') IS NOT NULL" | grep -qx t

echo "[OK] migrations applied and critical invariants verified"
