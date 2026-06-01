# CLAUDE.md — Pixela (Visual Regression Platform)

> Этот файл — постоянный контекст для всех сессий Claude Code в этом репозитории.
> Держи его в корне. Не удаляй. Обновляй раздел "Current State" по мере прогресса.

## Что мы строим

**Pixela** — самостоятельно размещаемая (self-hosted) платформа визуального регрессионного
тестирования. Аналог Argos / Visual Regression Tracker. Принимает скриншоты из Playwright-тестов
(через CI), сравнивает с baseline, показывает diff в веб-дашборде, даёт workflow approve/reject,
хранит историю прогонов по CI-джобам, репортит статус в GitLab MR.

Полная спека лежит в `./docs/spec/` (или там, куда ты положил этот пакет). Всегда сверяйся с ней.

## Ключевые архитектурные инварианты (НЕ нарушать без явного разрешения)

1. **Git-native baseline.** Baseline-скриншоты живут в git тестируемого проекта как файлы.
   "Approve" = подготовка коммита с обновлёнными снапшотами. Мы НЕ реализуем серверный
   merge-base baseline resolution. Если задача подталкивает к нему — остановись и спроси.
2. **Diff — это дешёвая часть.** Не переусложняй. pixelmatch + sharp закрывают 90%. odiff — опция.
3. **Stateless ingestion, async diff.** API только принимает и кладёт в очередь. Сравнение — в воркере (BullMQ).
4. **Content-addressable storage.** Картинки хранятся по sha256-хэшу содержимого. Дедупликация обязательна.
5. **Изоляция проектов.** Каждый запрос аутентифицируется API-ключом проекта. Данные проектов не пересекаются.
6. **Детерминизм важнее фич.** Тестовый инструмент, который иногда врёт, хуже отсутствия инструмента.
   Любая логика сравнения/резолва покрывается тестами на краевые случаи.

## Стек

> ⚠️ Бэкенд переписан на **Go** (ADR `docs/adr/0001-backend-language-go.md`). Конвенции и rulebook —
> `docs/architecture/go-backend.md` (придерживаться при любом Go-коде). Спека `02-architecture.md`/
> `backend/tech-decisions.md` описывает прежний NestJS-стек — для бэка она заменена этим ADR; инварианты,
> data-model и API-контракт остаются в силе.

- Backend: **Go 1.26** (один бинарь, subcommands `serve|worker|migrate`) · **Huma v2 на chi** (code-first
  OpenAPI 3.1) · **pgx/v5 + sqlc + Atlas** · **River** (Postgres-транзакционная очередь)
- Diff: pure-Go **orisano/pixelmatch + image/png** (`CGO_ENABLED=0`); libvips — за seam'ом, не в v1
- Storage: **S3-совместимое** (MinIO для self-host), CAS по sha256 (по *декодированным* пикселям)
- Frontend: **Angular** (standalone + signals), **Angular CDK**. Дизайн-референс (макеты + дизайн-система,
  тёмная тема `#0E0E10`/акцент `#6E8AFA`, Geist) — `docs/design/` (индекс: `docs/design/PIXELA-DESIGN.md`).
  Прибегай к нему при любой вёрстке/UI (Фаза 4+).
- Playwright: custom **reporter** (`@pixela/playwright-reporter`, TS) + тонкий SDK; контракт-типы генерятся
  из OpenAPI бэка (`openapi-typescript` → `packages/shared`)
- Redis: **только dashboard-сессии** (не очередь). Infra: **Docker Compose** + **Traefik**

## Стиль кода и работы

- Backend (Go): см. `docs/architecture/go-backend.md` — lightweight hexagonal, hand-wired DI, `internal/core`
  без зависимостей, Huma-struct-валидация на каждом endpoint, errors as values. Frontend/SDK: TypeScript strict, без `any` без обоснования.
- Frontend: standalone components, signals для состояния, OnPush change detection, без NgModules.
- Тесты: Go `testing` + Testcontainers (`-race`, goleak) для бэка; Playwright для e2e. Каждый PR — зелёные тесты.
- Коммиты атомарные, conventional commits (`feat:`, `fix:`, `chore:`...).
- Никаких секретов в коде. Всё через env. `.env.example` всегда актуален.

## Current State (обновляй это!)

- [x] **Фаза 0: каркас + миграции + smoke** ✅ (детали ниже)
- [ ] Фаза 1: ingestion + storage
- [ ] Фаза 2: diff pipeline
- [ ] Фаза 3: Playwright reporter
- [ ] Фаза 4: дашборд + review UI
- [ ] Фаза 5: approve + GitLab
- [ ] Фаза 6: уведомления
- [ ] Фаза 7: деплой

### Зафиксированные решения (старт проекта)

- **Единый монорепо `pixela`** (pnpm workspaces, рекомендация спеки B-01): `apps/api` (бэк),
  `apps/web` (Angular-дашборд), `packages/sdk`, `packages/shared`. Контракт `04-api-contract.md`
  меняется атомарно в пределах одного репо; фронт переиспользует типы из `@pixela/shared`.
  _(Изначально были два репо `pixela-back`/`pixela-ui` — объединены в `pixela` по решению владельца.)_
- **Baseline Mode A** (Playwright владеет baseline-файлами; Pixela — слой ревью) — дефолт, финализация к Фазе 3/5.
- **Сессии дашборда — server-side в Redis** (B-03), секрет `SESSION_SECRET`.
- **Smoke — Testcontainers** (хермётично), а не против поднятого compose.

### Фаза 0 — что готово (бэкенд на Go)

- `apps/api` — **Go-модуль**, один бинарь `pixela` с subcommands **`serve | worker | migrate | openapi`**
  (stdlib flag, не `API_MODE`). Layout: `cmd/pixela`, `internal/{app,config,core,httpapi,db,redis,storage,
  queue,diff,...}`. Конвенции: `docs/architecture/go-backend.md`.
- Стек: **Huma v2 на chi** (OpenAPI 3.1), **pgx/v5 + sqlc** (схема `db/schema.sql` 1:1 по `03-data-model`),
  **River** (Postgres-очередь), pure-Go diff-seam, **log/slog**, **caarlos0/env**. CGO_ENABLED=0.
- `pixela migrate` — применяет схему (embedded, идемпотентно) + River-таблицы. `pixela openapi` — эмитит спеку.
- **`/healthz`** (liveness) + **`/readyz`** (реальная проверка Postgres+Redis+MinIO) — заменяют одиночный `/health`.
- Зелёный **Testcontainers-smoke** (`apps/api/test/smoke_test.go`, `-tags=integration`, `-race`): migrate на
  чистой БД → `serve` → `/readyz` 200 со всеми up → graceful shutdown. Гейт `verify-go/scripts/gate.sh` зелёный.
- `apps/web` — Angular 21 shell (без изменений). `packages/sdk` (TS reporter) + `packages/shared` (типы из OpenAPI).
- CI (`.gitlab-ci.yml`): Go jobs (golangci-lint v2 + vet, build, test -race, integration DinD) + web-build.
- Dockerfile: multi-stage → distroless/static, один образ / два `command:`.

### Как запустить (кратко; полностью — в README.md)

```bash
pnpm dev:infra                  # postgres + redis(sessions) + minio (host-порты параметризуемы)
cp .env.example .env && set -a && . ./.env && set +a
pnpm migrate                    # cd apps/api && go run ./cmd/pixela migrate
pnpm dev:api                    # go run ./cmd/pixela serve → curl :3000/readyz
cd apps/api && go test -tags integration -race ./test/...   # Testcontainers smoke
```

## Чего НЕ делать

- НЕ хранить картинки в Postgres (только метаданные; блобы в S3/MinIO).
- НЕ делать синхронный diff в HTTP-запросе.
- НЕ изобретать merge-base baseline resolution (см. инвариант #1).
- НЕ тащить тяжёлые UI-киты в Angular (Material целиком и т.п.) — только CDK + свои компоненты.
- НЕ использовать localStorage/sessionStorage в дашборде для критичного состояния (только кэш).
- НЕ коммитить node_modules, .env, тестовые артефакты, baseline PNG самого Pixela.

## Когда сомневаешься

Спроси. Особенно по: baseline-логике, схеме БД (миграции необратимы в проде),
формату API-контракта (его потребляет SDK), и любым решениям, влияющим на детерминизм diff.
