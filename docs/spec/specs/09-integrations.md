# 09 — Integrations

> GitLab first. Telegram/Slack — nice-to-have. Все интеграции — через REST, изолированы в `notifications/`.

## GitLab CI

### Передача контекста из пайплайна (F-17)

Playwright reporter автоматически читает GitLab CI predefined variables:

| Pixela поле | GitLab CI переменная |
|-------------|----------------------|
| `branch` | `CI_COMMIT_REF_NAME` |
| `commitSha` | `CI_COMMIT_SHA` |
| `ciBuildId` | `CI_PIPELINE_ID` |
| `ciJobUrl` | `CI_JOB_URL` |
| `mrIid` | `CI_MERGE_REQUEST_IID` (если pipeline для MR) |
| `parallelTotal` | `CI_NODE_TOTAL` (при parallel) |
| project key | из `PIXELA_PROJECT_KEY` (secret CI/CD variable) |

### Статус билда в MR (F-41)

После завершения diff Pixela ставит **commit status** через GitLab API:
```
POST /projects/:id/statuses/:sha
{ state: "success" | "pending" | "failed", name: "pixela/visual",
  target_url: "<ссылка на билд в дашборде>", description: "12 changes need review" }
```
- `COMPARING` → `pending`
- `PASSED` → `success`
- `REVIEW_REQUIRED` → `failed` (или `pending`, если не хотим блокировать merge) — конфигурируемо
- `REJECTED` → `failed`

> Это позволяет завязать merge-блокировку на визуальное ревью через GitLab merge request approval rules.

### Комментарий в MR (F-42)

Опционально Pixela постит/обновляет комментарий в MR со сводкой:
```
POST /projects/:id/merge_requests/:iid/notes
"🖼 Pixela: 12 changed, 3 new, 1 removed. [Review →](<ссылка>)"
```
- Идемпотентность: хранить note id, обновлять существующий комментарий, не плодить новые.

### Токен и права

- Нужен GitLab access token (project/group) с правами: `api` (статусы, комментарии) и `write_repository`
  (если делаем commit/MR для baseline, F-32 вариант 2/3).
- Хранить как секрет в env Pixela (`GITLAB_TOKEN`), не в БД проектов в открытом виде.

### Пример `.gitlab-ci.yml` для тестируемого проекта

```yaml
visual-tests:
  image: mcr.microsoft.com/playwright:v1.xx-jammy
  stage: test
  parallel: 4
  script:
    - npm ci
    - npx playwright test --shard=$CI_NODE_INDEX/$CI_NODE_TOTAL
  variables:
    PIXELA_API_URL: "https://pixela.example.com"
    # PIXELA_PROJECT_KEY задан как masked CI/CD variable
  artifacts:
    when: always
    paths: [playwright-report/]
```

## GitLab OAuth (F-43)

Вход в дашборд Pixela через GitLab:
- `GET /api/v1/auth/gitlab` → редирект на GitLab authorize.
- `GET /api/v1/auth/gitlab/callback` → обмен code на token, апсерт User по `gitlabId`, выдача сессии.
- Конфиг: `GITLAB_OAUTH_CLIENT_ID`, `GITLAB_OAUTH_CLIENT_SECRET`, `GITLAB_BASE_URL`.

## Telegram (F-47)

Уведомления о событиях билда в Telegram-чат/канал.

- Через Bot API: `POST https://api.telegram.org/bot<token>/sendMessage`.
- Конфиг per-project ИЛИ глобальный: `TELEGRAM_BOT_TOKEN`, `chatId` (на проект).
- События: «билд требует ревью» (REVIEW_REQUIRED), «билд passed после ревью», опц. «ошибка обработки».
- Формат сообщения: проект, ветка, счётчики, ссылка на билд.
- Тихие апдейты: не спамить на каждый снапшот — одно сообщение на смену статуса билда.

```
🖼 Pixela • myproject
Ветка: feature/seatmap
Требуется ревью: 12 changed, 3 new
→ https://pixela.example.com/builds/<id>
```

## Slack (F-48)

- Через Incoming Webhook URL (проще) или Slack App.
- Конфиг: `SLACK_WEBHOOK_URL` per-project.
- Те же события, что и Telegram. Формат — Slack blocks (заголовок + ссылка-кнопка).

## Архитектура notifications-модуля

```
notifications/
├── notifications.service.ts     # оркестратор: на событие билда → разослать по включённым каналам
├── channels/
│   ├── gitlab-status.channel.ts
│   ├── gitlab-comment.channel.ts
│   ├── telegram.channel.ts
│   └── slack.channel.ts
└── notification-config.ts        # какие каналы включены per-project
```

- Каналы — за общим интерфейсом `NotificationChannel { notify(event): Promise<void> }`.
- Отправка — fire-and-forget с логированием ошибок; падение уведомления НЕ ломает обработку билда.
- Дебаунс/идемпотентность: один build status change = одна нотификация на канал.

## Webhook-исходящие (F-56, Could)

- `POST <configured-url>` с подписью HMAC на события `build.completed` / `build.review_required`.
- Для интеграций, которых нет из коробки. Пост-MVP.
