#!/usr/bin/env bash
set -euo pipefail
umask 077

# First run: prepare secrets and keys. Second run: deploy with offline-signed trust bundle.
REPO_DIR="${VPNX3_INSTALL_DIR:-/opt/vpnx3}"
SECRETS_FILE="$REPO_DIR/.env.production"
TRUST_DIR="$REPO_DIR/private/trust"
COMPOSE_FILE="$REPO_DIR/docker-compose.production.yml"

fail(){ printf '[FAIL] %s\n' "$*" >&2; exit 1; }
info(){ printf '[VPNX3] %s\n' "$*"; }
[[ "${EUID}" -eq 0 ]] || fail "Запустите от root."
[[ "$(uname -s)" == "Linux" ]] || fail "Требуется Linux."
MODE="${1:-}"
[[ "$MODE" == "prepare" || "$MODE" == "deploy" || "$MODE" == "status" ]] ||
  fail "Использование: bash scripts/install-control-production.sh prepare|deploy|status"

cd "$REPO_DIR" 2>/dev/null || fail "Сначала клонируйте репозиторий в $REPO_DIR"
[[ -f "$COMPOSE_FILE" ]] || fail "Не найден docker-compose.production.yml"
command -v python3 >/dev/null || fail "Требуется python3"

if [[ "$MODE" == "prepare" ]]; then
  [[ ! -e "$SECRETS_FILE" ]] || fail "Секреты уже существуют. Установщик не перезаписывает ключи."
  read -r -p "Email владельца: " OWNER_EMAIL
  [[ "$OWNER_EMAIL" == *@* ]] || fail "Некорректный email"
  read -r -s -p "Пароль владельца (не менее 14 символов): " OWNER_PASSWORD
  printf '\n'
  [[ "${#OWNER_PASSWORD}" -ge 14 ]] || fail "Пароль слишком короткий"
  install -d -m 0700 "$TRUST_DIR" "$REPO_DIR/private/artifacts"
  chown 65532:65532 "$REPO_DIR/private/artifacts"
  export OWNER_EMAIL OWNER_PASSWORD REPO_DIR
  python3 - <<'PY'
import base64,os,pathlib,secrets,urllib.parse
root=pathlib.Path(os.environ["REPO_DIR"])
def seed():
    return base64.urlsafe_b64encode(secrets.token_bytes(32)).decode().rstrip("=")
db=secrets.token_urlsafe(42)
url="postgres://vpnx3:"+urllib.parse.quote(db,safe="")+"@127.0.0.1:5432/vpnx3?sslmode=disable"
values={
 "VPNX3_ENV":"production",
 "VPNX3_HTTP_ADDR":":8080",
 "VPNX3_LOG_LEVEL":"info",
 "VPNX3_DATABASE_URL":url,
 "VPNX3_POSTGRES_PASSWORD":db,
 "VPNX3_BOOTSTRAP_OWNER_EMAIL":os.environ["OWNER_EMAIL"],
 "VPNX3_BOOTSTRAP_OWNER_PASSWORD":os.environ["OWNER_PASSWORD"],
 "VPNX3_CONFIG_SIGNING_KEY":seed(),
 "VPNX3_ACCESS_SIGNING_KEY":seed(),
 "VPNX3_RELEASE_SIGNING_KEY":seed(),
 "VPNX3_TRUST_ROOT_PUBLIC_KEY":"",
 "VPNX3_TRUST_BUNDLE_FILE":"/etc/vpnx3/trust-bundle.json",
 "VPNX3_TRUST_BUNDLE_HOST_FILE":str(root/"private/trust/trust-bundle.json"),
 "VPNX3_ARTIFACT_STORAGE":"local",
 "VPNX3_ARTIFACT_DIR":"/var/lib/vpnx3/artifacts",
 "VPNX3_ARTIFACT_HOST_DIR":str(root/"private/artifacts"),
 "VPNX3_ADMIN_SESSION_BINDING":"user-agent",
}
for k,v in values.items():
    if "\n" in v or "\r" in v:
        raise SystemExit("newlines prohibited in environment variables")
    if k=="VPNX3_BOOTSTRAP_OWNER_PASSWORD" and ("#" in v or "'" in v or '"' in v or "$" in v):
        raise SystemExit("Use password without #, quotes, or $ for dotenv compatibility")
dest=root/".env.production"
dest.write_text("".join(f"{k}={v}\n" for k,v in values.items()))
dest.chmod(0o600)
print("Секреты созданы, доступ root-only.")
PY
  info "Сейчас НЕ запускайте сервер: сначала подпишите trust bundle автономным корневым ключом."
  info "Получите открытые ключи из seeds на автономной машине; подробности: docs/PRODUCTION_INSTALL.md."
  exit 0
fi

if [[ "$MODE" == "status" ]]; then
  [[ -f "$SECRETS_FILE" ]] || fail "Не подготовлено окружение."
  docker compose --env-file "$SECRETS_FILE" -f "$COMPOSE_FILE" ps
  exit 0
fi

[[ -f "$SECRETS_FILE" ]] || fail "Сначала выполните prepare."
[[ -f "$TRUST_DIR/trust-bundle.json" ]] || fail "Отсутствует автономно подписанный trust bundle."
[[ -f "$TRUST_DIR/root-public-key.txt" ]] || fail "Отсутствует открытый корневой ключ."
chmod 600 "$SECRETS_FILE" "$TRUST_DIR/trust-bundle.json" "$TRUST_DIR/root-public-key.txt"
ROOT_PUB="$(tr -d '\r\n' <"$TRUST_DIR/root-public-key.txt")"
[[ "$ROOT_PUB" =~ ^[A-Za-z0-9_-]{43}$ ]] || fail "Неверный формат Ed25519 root public key."
export ROOT_PUB SECRETS_FILE
python3 - <<'PY'
import os,pathlib
p=pathlib.Path(os.environ["SECRETS_FILE"])
rows=p.read_text().splitlines()
rows=[line for line in rows if not line.startswith("VPNX3_TRUST_ROOT_PUBLIC_KEY=")]
rows.append("VPNX3_TRUST_ROOT_PUBLIC_KEY="+os.environ["ROOT_PUB"])
p.write_text("\n".join(rows)+"\n")
p.chmod(0o600)
PY
command -v docker >/dev/null || fail "Установите Docker Engine + Compose v2."
docker compose version >/dev/null || fail "Нужен docker compose v2."
info "Проверка конфигурации и сборка..."
docker compose --env-file "$SECRETS_FILE" -f "$COMPOSE_FILE" config --quiet
docker compose --env-file "$SECRETS_FILE" -f "$COMPOSE_FILE" build migrate controlplane
info "Применение миграций и запуск..."
docker compose --env-file "$SECRETS_FILE" -f "$COMPOSE_FILE" up -d --wait postgres
docker compose --env-file "$SECRETS_FILE" -f "$COMPOSE_FILE" run --rm migrate
docker compose --env-file "$SECRETS_FILE" -f "$COMPOSE_FILE" up -d --no-deps controlplane
for i in $(seq 1 30); do
  if curl --silent --show-error --fail --max-time 3 http://127.0.0.1:8080/health/live >/dev/null; then
    info "Control Plane отвечает на localhost:8080"
    info "Публичный TLS reverse proxy и внешние ноды настраиваются отдельно."
    exit 0
  fi
  sleep 2
done
docker compose --env-file "$SECRETS_FILE" -f "$COMPOSE_FILE" ps >&2
fail "Control Plane не прошёл liveness; проверьте docker compose logs controlplane."
