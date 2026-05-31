# Infra — Deployment (reverse-proxy, TLS, бэкапы, обновления)

> Эксплуатационный справочник. Реализация — Agent 11. Соответствует `specs/11-nonfunctional.md`.

## Reverse-proxy и TLS

- **Traefik** (или nginx) — единственный публично слушающий сервис. Терминирует TLS, маршрутизирует:
  `Host(pixela.example.com)` → web на `/`, api на `/api`.
- TLS: Let's Encrypt через ACME (http-challenge) либо собственный сертификат (если закрытый контур/нет
  публичного DNS). Редирект http→https.
- Внутренние сервисы (postgres, redis, minio) слушают только во внутренней docker-сети, порты наружу не публикуются.
- MinIO-консоль (9001) недоступна публично (закрыть/basic-auth/VPN).

## Переменные окружения (прод)

Все секреты — из `.env.prod` (не в репо). `.env.prod.example` закоммичен с заглушками. Минимум:

```
# БД и очередь
DATABASE_URL=postgresql://pixela:${PG_PASSWORD}@postgres:5432/pixela
REDIS_URL=redis://redis:6379

# хранилище
S3_ENDPOINT=http://minio:9000
S3_BUCKET=pixela
S3_ACCESS_KEY=${MINIO_USER}
S3_SECRET_KEY=${MINIO_PASSWORD}
S3_FORCE_PATH_STYLE=true

# секреты приложения
SESSION_SECRET=...            # или JWT_SECRET
# интеграции (опционально)
GITLAB_TOKEN=...
GITLAB_BASE_URL=https://gitlab.example.com
GITLAB_OAUTH_CLIENT_ID=...
GITLAB_OAUTH_CLIENT_SECRET=...
TELEGRAM_BOT_TOKEN=...
SLACK_WEBHOOK_URL=...

# прочее
PUBLIC_URL=https://pixela.example.com
IMAGE_MAX_BYTES=10485760
PRESIGNED_TTL_SECONDS=3600
```

## Миграции при деплое

- Прод: `prisma migrate deploy` (НЕ `migrate dev`) — применяет уже сгенерированные миграции.
- Запускать как init-шаг перед стартом api (отдельный one-off контейнер или entrypoint api).
- Миграции необратимы в проде — ревьюить изменения схемы особенно тщательно (инвариант из CLAUDE.md).

## Обновление (rolling)

1. Pull новых образов (`pixela-api`, `pixela-web`).
2. Применить миграции (`prisma migrate deploy`).
3. Перезапустить api, worker, web. Postgres/Redis/MinIO не трогать.
4. Проверить healthchecks и один сквозной прогон.

> Порядок важен: миграции до новых образов, если они расширяют схему обратносовместимо; для несовместимых —
> двухфазный деплой (expand/contract). В v1 стремиться к обратносовместимым миграциям.

## Бэкап (F-40)

Главное, что нельзя потерять: **Postgres** (метаданные, статусы, история) и **MinIO** (все PNG: baseline,
new, diff). Без блобов история сравнений нерабочая.

`scripts/backup.sh` (cron, напр. ежедневно):

```bash
#!/usr/bin/env bash
set -euo pipefail
TS=$(date +%Y%m%d-%H%M%S)
DEST=/backups/$TS
mkdir -p "$DEST"

# 1. Postgres
docker exec pixela-postgres pg_dump -U pixela pixela | gzip > "$DEST/pixela-db.sql.gz"

# 2. MinIO bucket (через mc; настроить alias заранее)
mc mirror --overwrite minio/pixela "$DEST/minio-pixela"

# 3. ротация: хранить N последних
ls -1dt /backups/*/ | tail -n +15 | xargs -r rm -rf

# 4. (рекомендуется) синхронизировать /backups на внешнее хранилище/в другой регион
```

- Хранить бэкапы ВНЕ сервера (другой диск/хост/регион). Бэкап на том же диске не спасает от потери диска.
- Шифровать бэкапы, если содержат чувствительные скриншоты.

## Восстановление (проверить хотя бы раз!)

```bash
# 1. Поднять стек (или чистый стек на новом сервере)
docker-compose -f docker-compose.prod.yml up -d postgres redis minio

# 2. Восстановить БД
gunzip -c /backups/<TS>/pixela-db.sql.gz | docker exec -i pixela-postgres psql -U pixela pixela

# 3. Восстановить бакет
mc mirror --overwrite /backups/<TS>/minio-pixela minio/pixela

# 4. Поднять приложение
docker-compose -f docker-compose.prod.yml up -d api worker web traefik

# 5. Проверить /health и открыть исторический билд (блобы должны грузиться)
```

> Бэкап без проверенного восстановления — это надежда, а не гарантия. Прогнать восстановление на стейджинге.

## Наблюдаемость

- `/health` на api и worker (используется healthcheck'ами).
- Структурированные логи (JSON), без секретов (маскировать токены).
- Метрики (пост-MVP): длина очереди BullMQ, время diff-job, билды по статусам, размер бакета.
- Мониторить рост miniodata (PNG копятся); план ретенции diff-PNG старых билдов (`specs/11-nonfunctional.md`).

## Ретенция (пост-MVP, заложить мышление)

- diff-PNG старых, давно закрытых билдов можно чистить (они производны). baseline и историю approve — не трогать.
- Реализовать как периодическую задачу позже; в v1 — просто мониторить место и документировать.

## Безопасность эксплуатации (сводка)

- TLS обязателен; http→https.
- Внутренние сервисы не публикуются наружу; MinIO-консоль закрыта.
- Секреты только из env, не в образах/репо/логах.
- Rate-limit на ingestion и login (см. `specs/10-security-and-auth.md`).
- Регулярные обновления базовых образов (postgres/redis/minio/traefik) — патчи безопасности.
