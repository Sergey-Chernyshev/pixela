# Agent 03 — Storage Layer

**Фаза:** 1 · **Фичи:** F-06..F-08 · **Зависит от:** Agent 00, Agent 01

## Задача
Абстракция хранилища блобов поверх S3/MinIO: content-addressable хранение PNG по sha256, presigned URL на
чтение, проверка существования (для дедупликации). Это слой, на который опираются ingestion и diff.

## Контекст (прочитать)
- `specs/02-architecture.md` (storage), `specs/04-api-contract.md` (presigned URL), `specs/10-security-and-auth.md`.

## Что сделать
1. `StorageService` с интерфейсом, скрывающим S3-клиент (`@aws-sdk/client-s3` или `minio` SDK):
   - `exists(sha256): Promise<boolean>` — проверка наличия блоба (для needUpload).
   - `put(sha256, buffer): Promise<void>` — положить блоб по ключу = sha256 (idempotent: если есть — no-op).
   - `getPresignedUrl(sha256, ttl): Promise<string>` — короткоживущий URL на чтение.
   - `getBuffer(sha256): Promise<Buffer>` — скачать блоб (для diff-воркера).
2. Конфиг из env: `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_REGION`, `S3_FORCE_PATH_STYLE` (true для MinIO).
3. Инициализация бакета при старте (создать, если нет).
4. Ключи объектов = чистый sha256 (опц. префикс-шардинг `ab/cd/<sha>` для распределения, если решишь).

## Acceptance criteria
- [ ] `put` с одинаковым sha дважды не дублирует объект и не падает.
- [ ] `exists` корректно различает наличие/отсутствие (основа дедупликации F-07).
- [ ] `getPresignedUrl` отдаёт рабочий URL, истекающий через TTL.
- [ ] `getBuffer` скачивает ровно тот контент, что был положен (round-trip тест).
- [ ] Работает против MinIO из `docker-compose.dev.yml` (path-style).
- [ ] Тесты: round-trip put/get, exists true/false, идемпотентность put.

## Не делать
- Не хранить метаданные картинок здесь (это Prisma `Image`, Agent 01/02). Storage — только байты.
- Не давать фронту прямой доступ к MinIO мимо presigned.
