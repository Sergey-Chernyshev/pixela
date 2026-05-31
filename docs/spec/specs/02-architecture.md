# 02 — Architecture

## Компоненты (высокоуровнево)

```mermaid
flowchart TB
    subgraph CI["GitLab CI (твой пайплайн)"]
        PW["Playwright тесты<br/>@pixela/playwright-reporter"]
    end

    subgraph Pixela["Pixela (self-hosted, docker-compose)"]
        API["API (NestJS)<br/>ingestion + REST"]
        Q["Queue (BullMQ / Redis)"]
        W["Diff Worker (NestJS)<br/>pixelmatch / odiff / sharp"]
        DB[("PostgreSQL<br/>метаданные, билды, статусы")]
        S3[("MinIO / S3<br/>PNG блобы, content-addressable")]
        FE["Angular Dashboard<br/>review UI"]
        NOTIF["Notifier<br/>Telegram / Slack / GitLab status"]
    end

    PW -->|"POST скриншоты + метаданные"| API
    API -->|"кладёт блобы"| S3
    API -->|"пишет build/snapshot rows"| DB
    API -->|"enqueue diff job"| Q
    Q --> W
    W -->|"читает baseline + new из"| S3
    W -->|"пишет diff-результат, статусы"| DB
    W -->|"кладёт diff PNG"| S3
    W -->|"триггерит"| NOTIF
    NOTIF -->|"pipeline status / MR comment"| CI
    FE -->|"REST: билды, diff, approve"| API
    API -->|"читает"| DB
    API -->|"presigned URL"| S3
```

## Поток данных: жизненный цикл билда

```mermaid
sequenceDiagram
    participant CI as GitLab CI
    participant API as Pixela API
    participant S3 as MinIO
    participant Q as Queue
    participant W as Diff Worker
    participant DB as Postgres
    participant FE as Dashboard

    CI->>API: POST /builds (project, branch, commit, ciBuildId)
    API->>DB: create Build (status=running)
    API-->>CI: { buildId, uploadToken }
    loop каждый скриншот
        CI->>API: POST /builds/:id/snapshots (name, browser, viewport, imageHash)
        API->>S3: проверить/загрузить блоб по hash
        API->>DB: create Snapshot (status=pending)
        API->>Q: enqueue diff job (snapshotId)
    end
    CI->>API: PATCH /builds/:id (finalize)
    API->>DB: Build.status = comparing
    Q->>W: diff job
    W->>DB: найти baseline для (project, branch, name, browser, viewport)
    W->>S3: скачать baseline + new
    W->>W: pixelmatch → diffRatio, diff PNG
    alt diffRatio == 0
        W->>DB: Snapshot.status = unchanged
    else baseline отсутствует
        W->>DB: Snapshot.status = new
    else есть различие
        W->>S3: загрузить diff PNG
        W->>DB: Snapshot.status = changed (diffRatio, diffImageHash)
    end
    W->>DB: если все snapshots обработаны → Build.status = (passed | review_required)
    W->>API: триггер notify
    FE->>API: GET /builds/:id (polling или SSE)
    FE->>API: POST /snapshots/:id/approve
```

## Сервисы в docker-compose

| Сервис | Образ/основа | Роль | Порт (внутр.) |
|--------|--------------|------|---------------|
| `pixela-api` | node (NestJS) | HTTP API + ingestion | 3000 |
| `pixela-worker` | node (NestJS) | diff jobs consumer | — |
| `pixela-web` | node→static (Angular) или nginx | дашборд | 80 |
| `postgres` | postgres:16 | БД | 5432 |
| `redis` | redis:7 | очередь BullMQ | 6379 |
| `minio` | minio/minio | S3-совместимое хранилище | 9000/9001 |
| `traefik` | traefik | reverse-proxy + TLS | 80/443 |

API и worker — один и тот же кодовый модуль, запускаемый в двух режимах (`API_MODE=http` / `API_MODE=worker`).
Это упрощает деплой и переиспользование Prisma-клиента и сервисов.

## Почему async diff (а не синхронно в запросе)

Билд может содержать сотни скриншотов. Синхронное сравнение в HTTP-хендлере: (1) держит соединение
минутами, (2) не масштабируется горизонтально, (3) падает по таймауту. Очередь BullMQ позволяет:
параллелить воркеры, ретраить упавшие jobs, изолировать тяжёлую CPU-работу (pixelmatch/sharp) от API.

## Границы модулей (NestJS feature modules)

```
src/
├── main.ts                  # bootstrap, переключение http/worker по env
├── app.module.ts
├── projects/                # CRUD проектов, API-ключи
├── builds/                  # создание/финализация билдов, агрегатный статус
├── snapshots/               # приём скриншотов, статусы, approve/reject
├── storage/                 # абстракция S3/MinIO, CAS по sha256, presigned URLs
├── diff/                    # diff engine, очередь, воркер-процессор
├── baseline/                # резолв baseline (git-native), история версий
├── notifications/           # Telegram, Slack, GitLab status/comment
├── auth/                    # API-ключи (для CI) + сессии/OAuth (для дашборда)
└── common/                  # DTO, guards, фильтры, утилиты
```

## Ключевые технические инварианты (повтор из CLAUDE.md)

- Блобы в S3, метаданные в Postgres. Никогда не наоборот.
- Ingestion stateless; diff async.
- CAS: один и тот же PNG хранится один раз (ключ = sha256 содержимого).
- Каждый внешний запрос проходит guard аутентификации (API-ключ или сессия).
