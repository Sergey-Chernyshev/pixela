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

- Backend: **NestJS** + **Prisma** + **PostgreSQL** + **BullMQ** (Redis)
- Diff: **pixelmatch** (default), **sharp** (обработка), **odiff** (опц.)
- Storage: **S3-совместимое** (MinIO для self-host), CAS по sha256
- Frontend: **Angular** (standalone + signals), **Angular CDK**
- Playwright: custom **reporter** (`@pixela/playwright-reporter`) + тонкий SDK
- Infra: **Docker Compose** + **Traefik**

## Стиль кода и работы

- TypeScript strict mode везде. Без `any` без явного комментария-обоснования.
- Backend: модульная структура NestJS (feature modules). DTO + class-validator на каждом endpoint.
- Frontend: standalone components, signals для состояния, OnPush change detection, без NgModules.
- Тесты: Vitest/Jest для backend unit, Playwright для e2e. Каждый PR — зелёные тесты.
- Коммиты атомарные, conventional commits (`feat:`, `fix:`, `chore:`...).
- Никаких секретов в коде. Всё через env. `.env.example` всегда актуален.

## Current State (обновляй это!)

- [ ] Фаза 0: каркас + миграции + smoke
- [ ] Фаза 1: ingestion + storage
- [ ] Фаза 2: diff pipeline
- [ ] Фаза 3: Playwright reporter
- [ ] Фаза 4: дашборд + review UI
- [ ] Фаза 5: approve + GitLab
- [ ] Фаза 6: уведомления
- [ ] Фаза 7: деплой

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
