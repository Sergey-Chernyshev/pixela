# Agent 00 — Repo Scaffold

**Фаза:** 0 · **Фичи:** F-38 (частично) · **Зависит от:** —

## Задача
Создать каркас монорепозитория Pixela: backend (NestJS), frontend (Angular), SDK-заготовка, общий
тулинг, dev docker-compose, базовый CI-линт. Это фундамент, поверх которого строятся все фазы.

## Контекст (прочитать)
- `CLAUDE.md`, `specs/02-architecture.md`, `backend/tech-decisions.md`, `frontend/angular-decisions.md`.

## Что сделать
1. Монорепо на pnpm workspaces (или npm). Структура:
   ```
   apps/api      # NestJS (http + worker режимы)
   apps/web      # Angular standalone
   packages/sdk  # @pixela/playwright-reporter (заготовка)
   packages/shared # общие TS-типы (опц., если удобно зеркалить контракт)
   ```
2. Корневой `tsconfig.base.json` (strict), общий ESLint/Prettier, `.editorconfig`, `.nvmrc`.
3. NestJS-приложение `apps/api` с переключением режима по `API_MODE=http|worker` в `main.ts`.
4. Angular-приложение `apps/web` (standalone, без NgModules), пустой shell с роутингом.
5. `packages/sdk` — пустой пакет с корректным `package.json`, точкой входа, build-скриптом.
6. `docker-compose.dev.yml`: postgres:16, redis:7, minio. `.env.example` со всеми переменными из спек.
7. `.gitignore` (node_modules, .env, dist, артефакты, baseline PNG самого Pixela).
8. Корневой `README.md`: как поднять dev-окружение.
9. Базовый CI (GitLab `.gitlab-ci.yml`): install, lint, typecheck. (Тесты добавятся позже.)

## Acceptance criteria
- [ ] `pnpm install` ставит весь монорепо.
- [ ] `docker-compose -f docker-compose.dev.yml up` поднимает postgres+redis+minio без ошибок.
- [ ] `apps/api` стартует в http-режиме и в worker-режиме (по env), не падает.
- [ ] `apps/web` собирается и показывает пустой shell.
- [ ] lint/typecheck зелёные на всём репо.
- [ ] `.env.example` содержит ВСЕ переменные, упомянутые в спеках (БД, Redis, S3, секреты, GitLab, Telegram, Slack).

## Не делать
- Не реализовывать бизнес-логику (это следующие фазы).
- Не тащить тяжёлые зависимости «на будущее».
