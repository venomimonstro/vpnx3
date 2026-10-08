# Установка VPNX3 в производственном режиме

## Выявленная проблема

Обычный `docker-compose.yml` предназначен для локальной разработки: тестовый пароль и `development`. **Не используйте его на боевом сервере**.

Новый `docker-compose.production.yml`: PostgreSQL не публикует порт 5432; Control Plane слушает только `127.0.0.1:8080` хоста. Контейнеры приложения и миграций подключаются к сетевому пространству PostgreSQL и используют БД через loopback. Публичный TLS настраивается отдельно, после проверки домена/сертификата. Локальные артефакты хранятся вне контейнеров.

## Основной сервер, Debian/Ubuntu

Установите Docker Engine, Docker Compose v2, Python 3, curl и git из доверенных источников.

```bash
sudo mkdir -p /opt
sudo git clone https://github.com/venomimonstro/vpnx3.git /opt/vpnx3
cd /opt/vpnx3
sudo bash scripts/install-control-production.sh prepare
```

На этапе prepare создаются отдельные случайные подписывающие ключи, пароль БД и владелец. Секреты сохраняются в `/opt/vpnx3/.env.production`, права 0600. **Никогда не публикуйте этот файл или закрытый корневой ключ.**

## Автономный корень доверия

Прежде чем выполнять deploy, на **другой доверенной машине**:

1. Команда `prepare` выводит **только публичные** CONFIG/ACCESS/RELEASE ключи через `cmd/public-keys`. Перенесите их на автономную машину; сами seeds никогда не копируйте.
2. Сгенерируйте **отдельный** корневой Ed25519 seed на автономной машине.
3. Создайте подписанный пакет командой `go run ./cmd/trust-bundle -root-seed-file ROOT_SEED_FILE -config-public-key CONFIG_PUB -access-public-key ACCESS_PUB -release-public-key RELEASE_PUB -version 1 -out trust-bundle.json`.
4. Передайте **только** `trust-bundle.json` и корневой *публичный* ключ на сервер:

```text
/opt/vpnx3/private/trust/trust-bundle.json
/opt/vpnx3/private/trust/root-public-key.txt
```

Закрытый корневой ключ остаётся автономно. Не запускайте production без этого шага.

## Запуск и повторный запуск

```bash
sudo bash scripts/install-control-production.sh deploy
sudo bash scripts/install-control-production.sh status
```

После успешного liveness-проверки установщик удаляет первоначальный пароль владельца из файла окружения. `deploy` сначала проверяет конфигурацию, затем собирает образы, применяет миграции и запускает Control Plane. При повторном запуске существующие ключи и данные не перегенерируются. Публичный HTTPS не включается автоматически: он требует действительного сертификата и отдельно настроенного обратного прокси.

Для присоединения worker-ноды используйте существующий `scripts/install-worker-node.sh` с однократным enrollment token, SHA-256 бинарников, подписанной цепочкой доверия и TLS Control Plane.

## До коммерческого запуска

Прогоните вручную `scripts/predeploy-check.sh`, `scripts/production-smoke.sh`, резервное копирование/восстановление, WireGuard handshake, пробные оплаты и сетевой failover. Отсутствие CI не отменяет обязательность проверок.

Это **начальный одноузловой производственный контур**, не высокодоступный кластер. Для HA нужны отдельный PostgreSQL standby, внешний резервный архив и как минимум две независимые сети для worker/ingress.
