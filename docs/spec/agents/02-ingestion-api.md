# Agent 02 — Ingestion API

**Фаза:** 1 · **Фичи:** F-01..F-05, F-34 · **Зависит от:** Agent 01, Agent 03 (storage)

## Задача
Реализовать endpoints приёма данных из CI: создание билда, двухшаговая заливка скриншотов с дедупликацией,
финализация, аутентификация по API-ключу. Это вход всех данных в систему.

## Контекст (прочитать)
- `specs/04-api-contract.md` (endpoints приёма — контракт), `specs/10-security-and-auth.md` (API-ключи),
  `specs/06-baseline-strategy.md` (что именно заливаем в режиме A).

## Что сделать
1. **Auth guard для ingestion**: `Authorization: ApiKey <key>` → хэш → поиск ApiKey → `projectId` в request-контекст.
   Обновлять `lastUsedAt`. Изоляция: запись только в проект ключа.
2. `POST /api/v1/builds` — создать Build (RUNNING) с метаданными (branch, commitSha, ciBuildId, ciJobUrl, mrIid, parallelTotal).
3. `POST /api/v1/builds/:id/snapshots` — шаг 1: принять метаданные + `imageSha256`; upsert Snapshot по
   `@@unique([buildId,name,browser,viewport])`; вернуть `{ snapshotId, needUpload }` (needUpload=false если блоб уже в S3).
4. `PUT /api/v1/images/:sha256` — шаг 2: принять binary PNG, проверить sha совпадает, проверить magic-байты
   и лимит размера, положить в storage (через Agent 03), создать/обновить Image-метаданные.
5. `PATCH /api/v1/builds/:id` (FINALIZE) — пометить COMPARING; вычислить REMOVED (baseline есть, снапшота нет);
   enqueue diff-jobs (через Agent 04 очередь). При шардинге — финализация по достижении parallelTotal.
6. DTO + class-validator на всё. Единый формат ошибок (коды из `04-api-contract.md`).

## Acceptance criteria
- [ ] Без валидного API-ключа любой ingestion-запрос → 401.
- [ ] Нельзя залить в чужой проект (ключ проекта A не пишет в проект B) → 403.
- [ ] Дедупликация: повторная заливка PNG с тем же sha → `needUpload:false`, байты не льются второй раз.
- [ ] Идемпотентность: повторная заливка того же снапшота (ретрай CI) НЕ создаёт дубль (upsert).
- [ ] Несовпадение sha загруженных байтов → 400 `SNAPSHOT_HASH_MISMATCH`.
- [ ] Превышение лимита размера → 400 `IMAGE_TOO_LARGE`; не-PNG → отклоняется.
- [ ] Финализация вычисляет REMOVED и кладёт diff-jobs в очередь.
- [ ] Тесты: дедуп, idempotent retry, hash mismatch, изоляция проектов, финализация+REMOVED.

## Не делать
- Не выполнять diff здесь (только enqueue). Diff — Agent 04.
- Не реализовывать merge-base (инвариант #1).
