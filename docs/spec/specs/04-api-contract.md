# 04 — API Contract

> REST API. JSON везде, кроме загрузки блобов (binary). Версионирование префиксом `/api/v1`.
> Этот контракт потребляет Playwright SDK и Angular-дашборд — менять синхронно с обоими.

## Аутентификация

Два контура:

1. **CI / SDK** — заголовок `Authorization: ApiKey <key>`. Идентифицирует проект. Только endpoints приёма.
2. **Дашборд** — сессионная cookie (после логина) или Bearer JWT. Доступ к чтению/approve по membership.

Guard проверяет: API-ключ → определяет `projectId`; сессия → определяет `userId` + проверяет membership на проект ресурса.

## Endpoints приёма (используются Playwright reporter)

### `POST /api/v1/builds`
Создать билд. Вызывается один раз в начале прогона (или reporter создаёт лениво на первом снапшоте).

```jsonc
// Request
{
  "branch": "feature/seatmap",
  "commitSha": "c3a1f9e",
  "ciBuildId": "gitlab-pipeline-12345",
  "ciJobUrl": "https://gitlab.example.com/.../jobs/678",
  "mrIid": "42",                 // опционально, для статуса в MR
  "parallelTotal": 4             // опц.: сколько шардов ожидается (для агрегации)
}
// Response 201
{ "buildId": "ckxyz...", "status": "RUNNING" }
```

### `POST /api/v1/builds/:buildId/snapshots`
Залить один скриншот. **Двухшаговый протокол ради CAS и дедупликации:**

Шаг 1 — объявить намерение по хэшу:
```jsonc
// Request (multipart ИЛИ json+отдельная загрузка)
{
  "name": "events-list--desktop",
  "browser": "chromium",
  "viewport": "1280x720",
  "imageSha256": "9f86d08...",   // хэш PNG, посчитанный на клиенте
  "width": 1280,
  "height": 720,
  "byteSize": 84211
}
// Response 200
{
  "snapshotId": "cksnap...",
  "needUpload": true             // false если блоб с таким sha уже есть в S3
}
```

Шаг 2 — если `needUpload: true`, залить байты:
```
PUT /api/v1/images/:sha256        (body: binary PNG, Content-Type: image/png)
→ 204 No Content
```
Сервер проверяет, что sha загруженного контента совпадает с заявленным (целостность).

> Это ядро дедупликации: неизменившиеся скриншоты имеют sha уже существующих baseline — их байты не льются повторно.

### `PATCH /api/v1/builds/:buildId`
Финализировать билд (все снапшоты залиты). Запускает вычисление `REMOVED` и enqueue diff.

```jsonc
// Request
{ "status": "FINALIZE" }
// Response 200
{ "buildId": "...", "status": "COMPARING" }
```

При шардинге: финализация срабатывает, когда пришли все `parallelTotal` шардов (счётчик в Redis/БД).

## Endpoints дашборда (используются Angular)

### `GET /api/v1/projects`
Список проектов текущего пользователя.

### `GET /api/v1/projects/:id/builds?branch=&status=&page=`
Лента билдов с фильтрами и пагинацией.
```jsonc
// Response
{
  "items": [{
    "id": "...", "branch": "feature/x", "commitSha": "c3a1f9e",
    "status": "REVIEW_REQUIRED",
    "counts": { "unchanged": 480, "changed": 12, "new": 3, "removed": 1 },
    "createdAt": "2026-05-31T10:00:00Z",
    "ciJobUrl": "..."
  }],
  "page": 1, "totalPages": 5
}
```

### `GET /api/v1/builds/:id`
Детали билда + список снапшотов с краткими метаданными (без тяжёлых URL — presigned по запросу).
```jsonc
{
  "id": "...", "branch": "...", "status": "REVIEW_REQUIRED",
  "snapshots": [{
    "id": "...", "name": "events-list--desktop",
    "browser": "chromium", "viewport": "1280x720",
    "status": "CHANGED", "diffRatio": 0.034
  }]
}
```

### `GET /api/v1/snapshots/:id`
Полные данные одного снапшота для review UI, включая presigned URLs на 3 изображения.
```jsonc
{
  "id": "...", "name": "...", "status": "CHANGED", "diffRatio": 0.034, "diffPixels": 3120,
  "images": {
    "baseline": "https://minio/.../baseline.png?X-Amz-...",  // null если NEW
    "new":      "https://minio/.../new.png?X-Amz-...",
    "diff":     "https://minio/.../diff.png?X-Amz-..."       // null если UNCHANGED/NEW
  },
  "history": [                                                 // лента прошлых approve
    { "buildId": "...", "action": "APPROVE", "user": "ivan", "at": "..." }
  ]
}
```

### `POST /api/v1/snapshots/:id/approve` и `/reject`
Принять/отклонить одно изменение.
```jsonc
// approve Response
{ "snapshotId": "...", "status": "APPROVED" }
```
Эффект approve:
1. создать `ApprovalEvent(APPROVE)`;
2. обновить/создать `Baseline` для (project, branch, name, browser, viewport) → новый `imageSha`;
3. пересчитать агрегатный статус билда (если не осталось CHANGED/NEW без решения → `PASSED`);
4. триггер git-native действия (см. `08-approve-workflow.md`) — подготовка коммита/MR.

### `POST /api/v1/builds/:id/approve-all` и `/reject-all`
Пакетный апрув/реджект всех CHANGED+NEW в билде. Возвращает счётчики.

### `GET /api/v1/builds/:id/events` (SSE)
Server-Sent Events для live-обновления статуса в дашборде во время `COMPARING`
(альтернатива polling). Отдаёт события `snapshot.updated`, `build.statusChanged`.

## Управление проектами и ключами

- `POST /api/v1/projects` — создать проект (owner = текущий юзер).
- `POST /api/v1/projects/:id/api-keys` — создать ключ; **сырой ключ в ответе один раз**.
- `DELETE /api/v1/api-keys/:id` — отозвать.
- `POST /api/v1/projects/:id/members` — добавить участника по email.

## Аутентификация (дашборд)

- `POST /api/v1/auth/login` — email+пароль → сессия/JWT.
- `GET /api/v1/auth/gitlab` + `GET /api/v1/auth/gitlab/callback` — OAuth-флоу (опц., но желателен).
- `POST /api/v1/auth/logout`.

## Ошибки

Единый формат:
```jsonc
{ "error": { "code": "SNAPSHOT_HASH_MISMATCH", "message": "Uploaded bytes do not match declared sha256" } }
```
Коды: `UNAUTHORIZED`, `FORBIDDEN_PROJECT`, `BUILD_NOT_FOUND`, `BUILD_ALREADY_FINALIZED`,
`SNAPSHOT_HASH_MISMATCH`, `IMAGE_TOO_LARGE`, `VALIDATION_ERROR`.

## Нефункциональные требования к API

- Все DTO валидируются (`class-validator`). Невалидный запрос → 400 `VALIDATION_ERROR`.
- Лимит размера PNG — конфигурируемый (дефолт 10 МБ), сверх → `IMAGE_TOO_LARGE`.
- Presigned URL живёт ограниченно (дефолт 1 час).
- Rate-limit на ingestion по проекту (защита от шального CI-цикла).
