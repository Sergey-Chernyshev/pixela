# Pixela — Self-Hosted Visual Regression Platform (Spec Package)

> Полный пакет спецификаций для постройки самостоятельно размещаемой visual regression платформы
> с помощью **Claude Code (Opus 4.8)** в режиме **goal / brainstorm**, стек **Angular + NestJS + Playwright**.

Кодовое имя проекта: **Pixela**. Это аналог Argos / Visual Regression Tracker, который ты разворачиваешь у себя.

---

## ⚠️ Прочти это первым (архитектурное решение, которое экономит недели)

Эта спека построена по **git-native baseline модели**, а НЕ по серверной модели Argos с
`merge-base resolution`. Это сознательный выбор:

- **baseline** хранится как PNG-файлы в git-репозитории тестируемого проекта (как нативный Playwright snapshot);
- **approve** = коммит обновлённых снапшотов (через CLI или кнопку, которая готовит коммит/MR);
- **дашборд Pixela** даёт то, чего нет у голого Playwright: красивое ревью diff, история прогонов по CI,
  3 режима сравнения, статусы в GitLab MR, уведомления;
- сложная и непредсказуемая часть (какой baseline брать на ветке `feature/x` относительно `main`)
  **делегирована git'у**, который решает её бесплатно и корректно при merge/rebase/squash.

**Почему так.** Серверный baseline-resolution (merge-base, параллельные прогоны, force-push, squash-merge,
per-screenshot approval state) — это месяц+ работы и бесконечный хвост багов на краевых случаях. Это ровно
та часть, где AI генерирует правдоподобный, но тонко неверный код, потому что в обучающих данных нет
канонической реализации. Git-native модель убирает этот класс задач целиком.

> Если тебе ОБЯЗАТЕЛЬНО нужен серверный baseline-resolution — см. `specs/06-baseline-strategy.md`,
> раздел "Server-side mode (NOT recommended)". По умолчанию мы его не строим.

---

## Как пользоваться этим пакетом с Claude Code

1. Скопируй весь распакованный каталог в корень нового репозитория (или в `./docs/spec/`).
2. Положи `CLAUDE.md` в корень репозитория — это контекст-якорь для всех сессий Claude Code.
3. Открой `prompts/00-goal-mode-master-prompt.md` — это стартовый промт для goal-режима. Вставь его целиком.
4. Claude Code построит **по фазам**. Каждая фаза = отдельная цель в goal-режиме, см. `prompts/01-phase-prompts.md`.
5. Используй `agents/*.md` как ТЗ для саб-агентов: каждый файл — самодостаточная задача с
   acceptance criteria, которую можно делегировать отдельному агенту.

### Рекомендуемый порядок постройки (фазы)

| Фаза | Что | Файлы | Зачем именно сейчас |
|------|-----|-------|---------------------|
| 0 | Каркас + миграции БД + 1 e2e smoke | `agents/00-*`, `agents/01-*` | Доказать пайплайн на минимуме |
| 1 | Ingestion API + storage (S3/MinIO, content-addressable) | `agents/02-*`, `agents/03-*` | Сердце приёма данных |
| 2 | Diff pipeline (pixelmatch/odiff, queue) | `agents/04-*` | Самая дешёвая часть, делается быстро |
| 3 | Playwright reporter + SDK | `agents/05-*`, `playwright/*` | То, ради чего всё затевалось |
| 4 | Angular дашборд: список билдов + review UI | `agents/06-*`, `agents/07-*`, `frontend/*` | Самая дорогая часть по UX |
| 5 | Approve workflow (git-native) + GitLab статусы | `agents/08-*`, `agents/09-*` | Замыкает цикл |
| 6 | Уведомления (Telegram/Slack) + полировка | `agents/10-*` | Nice-to-have |
| 7 | CI/CD самого Pixela + деплой | `agents/11-*`, `infra/*` | Когда есть что катить |

> Не строй фазы 4–7 спекулятивно. Доведи 0–3 до зелёного, посмотри, дальше тиражируй.

---

## Содержимое пакета

```
vrt-spec/
├── README.md                      ← ты здесь
├── CLAUDE.md                      ← контекст-якорь для Claude Code (копировать в корень репо)
├── specs/
│   ├── 01-overview-and-goals.md   ← что строим, зачем, границы scope
│   ├── 02-architecture.md         ← компоненты, диаграммы, потоки данных
│   ├── 03-data-model.md           ← схема БД (Prisma), сущности, состояния
│   ├── 04-api-contract.md         ← REST API endpoints, форматы, auth
│   ├── 05-features.md             ← ПОЛНЫЙ список фич с приоритетами (MoSCoW)
│   ├── 06-baseline-strategy.md    ← git-native baseline (ядро), почему так
│   ├── 07-diff-engine.md          ← движки сравнения, пороги, ignore-области
│   ├── 08-frontend-spec.md        ← Angular дашборд, review UI в деталях
│   ├── 09-integrations.md         ← GitLab CI, Telegram, Slack, webhooks
│   ├── 10-security-and-auth.md    ← аутентификация, API-ключи, изоляция проектов
│   ├── 11-nonfunctional.md        ← перформанс, масштаб, надёжность, бэкапы
│   └── 12-glossary.md             ← термины (baseline, build, snapshot, и т.д.)
├── prompts/
│   ├── 00-goal-mode-master-prompt.md  ← СТАРТОВЫЙ промт для goal-режима
│   ├── 01-phase-prompts.md            ← промт на каждую фазу
│   └── 02-brainstorm-prompts.md       ← промты для brainstorm-режима (открытые вопросы)
├── agents/
│   ├── 00-repo-scaffold.md
│   ├── 01-db-and-migrations.md
│   ├── 02-ingestion-api.md
│   ├── 03-storage-layer.md
│   ├── 04-diff-pipeline.md
│   ├── 05-playwright-reporter.md
│   ├── 06-frontend-shell.md
│   ├── 07-review-ui.md
│   ├── 08-approve-workflow.md
│   ├── 09-gitlab-integration.md
│   ├── 10-notifications.md
│   └── 11-deploy-and-ci.md
├── playwright/
│   ├── reporter-design.md         ← дизайн @pixela/playwright-reporter
│   ├── example-usage.md           ← как тестовый проект подключает Pixela
│   └── fixtures-and-determinism.md ← детерминизм скриншотов (твой kit)
├── backend/
│   └── tech-decisions.md          ← NestJS, Prisma, BullMQ обоснования
├── frontend/
│   └── angular-decisions.md       ← Angular архитектура, signals, состояние
├── infra/
│   ├── docker-compose.md          ← локальный и прод compose
│   └── deployment.md              ← reverse-proxy, TLS, бэкапы, обновления
└── skills/
    └── how-to-use-your-skills.md  ← как подключить твои Claude Code skills к проекту
```

---

## Стек (зафиксирован)

- **Backend:** NestJS (TypeScript), Prisma ORM, PostgreSQL, BullMQ (очереди на Redis)
- **Diff engine:** pixelmatch (default), odiff (опционально), sharp для обработки PNG
- **Storage:** S3-совместимое (MinIO для self-host), content-addressable по sha256
- **Frontend:** Angular (standalone components, signals), Angular CDK, без тяжёлых UI-китов
- **Playwright integration:** custom reporter + тонкий SDK
- **Infra:** Docker Compose, Traefik/nginx reverse-proxy
- **CI цель:** GitLab CI (но интеграция через REST, легко портируется)

> Стек выбран так, чтобы Claude Code имел максимум опоры в обучающих данных. Все решения с
> обоснованиями — в `backend/tech-decisions.md` и `frontend/angular-decisions.md`.

---

## Лицензия и происхождение

Это твой собственный проект. За референс взяты публичные архитектурные идеи Argos (MIT) и
Visual Regression Tracker (Apache-2.0). Никакой их код сюда не копируется — только концепции.
