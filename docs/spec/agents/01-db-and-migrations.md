# Agent 01 — DB & Migrations

**Фаза:** 0 · **Фичи:** схема данных · **Зависит от:** Agent 00

## Задача
Реализовать схему БД через Prisma строго по `specs/03-data-model.md`, первую миграцию, генерацию клиента,
подключение из api/worker, и health-проверку коннекта.

## Контекст (прочитать)
- `specs/03-data-model.md` (полный `schema.prisma` — это контракт), `specs/02-architecture.md`.

## Что сделать
1. `prisma/schema.prisma` точно по спеке: модели Image, Project, ApiKey, Build, Snapshot, Baseline,
   ApprovalEvent, User, Membership; все enum'ы; все индексы и `@@unique`.
2. `DATABASE_URL` из env. Первая миграция (`prisma migrate dev`), коммит миграции.
3. Генерация Prisma Client, обёртка-сервис в NestJS (`PrismaService` с подключением/отключением в lifecycle).
4. `/health` на api: проверяет реальный коннект к Postgres (простой query) и к Redis.
5. Сид-скрипт (опц.): создать demo-проект + API-ключ для локальной отладки.

## Acceptance criteria
- [ ] Миграция применяется на чистой БД без ошибок; повторный `migrate` идемпотентен.
- [ ] Все сущности/индексы/уникальные ограничения соответствуют `03-data-model.md`.
- [ ] `PrismaService` корректно коннектится/дисконнектится в lifecycle NestJS.
- [ ] `/health` возвращает 200 только если БД и Redis доступны; иначе 503.
- [ ] Smoke-тест: api стартует, видит БД и Redis (это «зелёный baseline пайплайна» Фазы 0).

## Не делать
- Не добавлять `BaselineVersion` (YAGNI, см. brainstorm B-01-ish в `03`). Только если я разрешу.
- Не менять имена полей/сущностей из глоссария.
