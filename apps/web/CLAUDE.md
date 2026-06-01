# CLAUDE.md — Pixela Web (`apps/web`)

> Вложенный контекст для **фронтенда** Pixela (Angular-дашборд) внутри монорепо `pixela`.
> Бэкенд — `apps/api`, общий каркас и **канонический** спек — в корне (`../../docs/spec/`,
> `../../CLAUDE.md` — источник правды). Этот файл — фронт-специфика поверх корневого контекста.

## Что это

Angular-дашборд платформы визуального регрессионного тестирования Pixela: список проектов и билдов,
детали билда, **review UI** (3 режима сравнения side-by-side / onion / slider + синхронный зум),
approve/reject. Потребляет REST API из `apps/api` (контракт — `../../docs/spec/specs/04-api-contract.md`).

## Инварианты фронтенда (НЕ нарушать без явного разрешения)

1. **Standalone + signals, без NgModules.** Состояние — сервисы-сторы на `signal`/`computed`; без NgRx в MVP.
2. **OnPush** везде.
3. **Только Angular CDK + свои компоненты.** НЕ тащить Angular Material целиком и тяжёлые UI-киты.
4. **Типы зеркалят контракт** `../../docs/spec/specs/04-api-contract.md` — можно переиспользовать
   `@pixela/shared` из того же монорепо (`packages/shared`). Presigned URL картинок приходят с сервера —
   фронт не ходит в MinIO напрямую.
5. **Критичное состояние НЕ в localStorage/sessionStorage** (только UI-преференции: тема, режим сравнения).
6. **Review UI — лицо продукта.** Перед UI-кодом Фазы 4 свериться с `frontend-design` skill
   (`../../docs/spec/skills/how-to-use-your-skills.md`). Синхронный зум — нерушимое требование (F-26).

> Архитектурные инварианты всего продукта (git-native baseline, блобы в S3, async diff, изоляция
> проектов, детерминизм) — в корневом `../../CLAUDE.md`. Они выше любых решений фронта.

## Стек

Angular 21 (standalone, signals, OnPush) · Angular CDK · SCSS · pnpm · Node 22.

## Current State

- [x] **Фаза 0: каркас фронта** ✅ — Angular standalone shell (header + lazy-routed Home), `ng build` зелёный.
- [ ] Фаза 4: dashboard shell + review UI (Agent 06/07) — основная работа фронта.

## Как запустить (из корня монорепо)

```bash
pnpm install                          # ставит весь воркспейс
pnpm --filter @pixela/web start       # ng serve → http://localhost:4200
pnpm --filter @pixela/web build       # прод-сборка в apps/web/dist/
```

API по умолчанию ожидается на `http://localhost:3000` (`apps/api`). Конфиг базового URL — к Фазе 4.

## Чего НЕ делать

- НЕ добавлять NgModules, NgRx, Angular Material целиком, Tailwind/сторонние дизайн-киты.
- НЕ ходить из фронта напрямую в MinIO; только presigned URL с API.
- НЕ хранить критичное состояние (сессия/данные) в localStorage.
- НЕ забегать в фичи Фаз, которые ещё не открыты владельцем.
