# Infra — Docker Compose (локальный и прод)

> Реализация — Agent 00 (dev) и Agent 11 (prod). Этот файл — справочник по составу и конфигурации.

## Два compose-файла

- `docker-compose.dev.yml` — только зависимости для локальной разработки (postgres, redis, minio).
  api/web запускаются с хоста (hot-reload).
- `docker-compose.prod.yml` — весь стек в контейнерах за Traefik с TLS.

## Dev (`docker-compose.dev.yml`)

Поднимает инфраструктуру; код бежит локально.

```yaml
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: pixela
      POSTGRES_PASSWORD: pixela
      POSTGRES_DB: pixela
    ports: ["5432:5432"]
    volumes: ["pgdata:/var/lib/postgresql/data"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U pixela"]
      interval: 5s
      timeout: 3s
      retries: 10

  redis:
    image: redis:7
    ports: ["6379:6379"]
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s

  minio:
    image: minio/minio
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: pixela
      MINIO_ROOT_PASSWORD: pixela-secret
    ports: ["9000:9000", "9001:9001"]
    volumes: ["miniodata:/data"]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/ready"]
      interval: 5s

volumes:
  pgdata:
  miniodata:
```

`.env.example` (dev) должен задавать: `DATABASE_URL=postgresql://pixela:pixela@localhost:5432/pixela`,
`REDIS_URL=redis://localhost:6379`, `S3_ENDPOINT=http://localhost:9000`, `S3_BUCKET=pixela`,
`S3_ACCESS_KEY=pixela`, `S3_SECRET_KEY=pixela-secret`, `S3_FORCE_PATH_STYLE=true`, и секреты-заглушки.

## Prod (`docker-compose.prod.yml`)

Весь стек в контейнерах. Ключевые отличия:

- **api** и **worker** — один образ, разный `API_MODE` (http / worker). worker масштабируется репликами.
- **web** — собранная статика Angular, отдаётся nginx-контейнером или напрямую Traefik.
- **traefik** — единственный публично слушающий сервис (80/443), TLS, маршрутизация.
- postgres/redis/minio — только внутри сети, порты наружу НЕ публикуются.
- MinIO-консоль (9001) — закрыта (или basic-auth/ограничена сетью).

Каркас:

```yaml
services:
  traefik:
    image: traefik:v3
    command:
      - "--providers.docker=true"
      - "--entrypoints.web.address=:80"
      - "--entrypoints.websecure.address=:443"
      - "--certificatesresolvers.le.acme.email=ops@example.com"
      - "--certificatesresolvers.le.acme.storage=/letsencrypt/acme.json"
      - "--certificatesresolvers.le.acme.httpchallenge.entrypoint=web"
    ports: ["80:80", "443:443"]
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock:ro"
      - "letsencrypt:/letsencrypt"

  api:
    image: pixela-api:latest
    environment:
      API_MODE: http
      DATABASE_URL: ${DATABASE_URL}
      REDIS_URL: ${REDIS_URL}
      S3_ENDPOINT: ${S3_ENDPOINT}
      # ... остальные секреты из .env.prod
    depends_on:
      postgres: { condition: service_healthy }
      redis: { condition: service_healthy }
      minio: { condition: service_healthy }
    labels:
      - "traefik.http.routers.api.rule=Host(`pixela.example.com`) && PathPrefix(`/api`)"
      - "traefik.http.routers.api.entrypoints=websecure"
      - "traefik.http.routers.api.tls.certresolver=le"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3000/health"]

  worker:
    image: pixela-api:latest      # тот же образ
    environment:
      API_MODE: worker
      # ... те же подключения
    depends_on:
      redis: { condition: service_healthy }
      postgres: { condition: service_healthy }
    deploy:
      replicas: 2                 # масштабирование diff

  web:
    image: pixela-web:latest
    labels:
      - "traefik.http.routers.web.rule=Host(`pixela.example.com`)"
      - "traefik.http.routers.web.entrypoints=websecure"
      - "traefik.http.routers.web.tls.certresolver=le"

  postgres:
    image: postgres:16
    environment: { POSTGRES_USER: pixela, POSTGRES_PASSWORD: ${PG_PASSWORD}, POSTGRES_DB: pixela }
    volumes: ["pgdata:/var/lib/postgresql/data"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U pixela"]
    # порт наружу НЕ публикуется

  redis:
    image: redis:7
    healthcheck: { test: ["CMD", "redis-cli", "ping"] }

  minio:
    image: minio/minio
    command: server /data --console-address ":9001"
    environment: { MINIO_ROOT_USER: ${MINIO_USER}, MINIO_ROOT_PASSWORD: ${MINIO_PASSWORD} }
    volumes: ["miniodata:/data"]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/ready"]
    # 9000/9001 наружу НЕ публикуются (доступ внутри сети; консоль закрыта)

volumes:
  pgdata:
  miniodata:
  letsencrypt:
```

## Замечания

- **Миграции при деплое**: применять `prisma migrate deploy` перед стартом api (init-контейнер или
  entrypoint-шаг). Не запускать `migrate dev` в проде.
- **Порядок старта**: api/worker зависят от healthy postgres+redis+minio (`depends_on: condition`).
- **CORS**: api разрешает origin дашборда (`pixela.example.com`).
- **Секреты**: все из `.env.prod` (не в репо). `.env.prod.example` — закоммичен с заглушками.
- **Обновление**: pull новых образов → `migrate deploy` → перезапуск api/worker/web. Postgres/MinIO не трогать.
- **Volumes**: pgdata и miniodata — на персистентном хранилище; они в зоне бэкапа (см. `deployment.md`).

## Целевой масштаб

docker-compose достаточно для ~50 проектов и билдов до ~500 скриншотов (см. `specs/11-nonfunctional.md`).
Kubernetes в v1 — избыточен (анти-паттерн: не усложнять оркестрацию).
