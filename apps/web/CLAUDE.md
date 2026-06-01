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

- [x] **Фаза 0: каркас фронта** ✅ — Angular standalone shell, `ng build` зелёный.
- [x] **Фаза 4: dashboard + review UI** ✅ — реализован по дизайн-бандлу (`docs/design/`, тёмная тема
  `#0E0E10`/индиго `#6E8AFA`/Geist), русский UI. Глобальная дизайн-система портирована в `src/styles.scss`
  (токены + компоненты + shell). Компоненты-раньше-страниц: `shared/` (`px-status`, `px-count-chips`),
  `layout/app-shell` (sidebar org/project + topbar). Spine `core/`: типобезопасный `ApiService` на
  `@pixela/shared`, `SessionService` (signal-сессия), `authGuard`/`guestGuard`, credentials-interceptor
  (cookie). 8 экранов на реальном API: **login** (email+пароль), **projects** (обзор орг. + health-бар/
  open-reviews/участники/last-build), **builds** (лента CI + пагинация + длительность), **build-detail**
  (грид снимков с presigned-миниатюрами), **review** (центр: 4 режима side/overlay/onion/curtain,
  **синхро-зум F-26**, клавиши A/R/1-4, presigned-картинки, история), **members** (роли + проверки),
  **baselines** (эталоны по веткам + миниатюры + staleness>90д), **activity** (орг-лента approve/reject,
  day-grouping). Dev-прокси `/api`→:3000. `ng build` + `ng test` зелёные.
  **Честно к API** (выбор владельца — вариант A: реальные ручки, без фабрикации, без смены контракта SDK):
  все агрегаты считаются из существующих строк (health=in-norm ratio последней сборки, open-reviews,
  members, activity из approval_events, baselines с presigned-thumbnail, длительность из finalized_at,
  миниатюры build-detail из MinIO). approve/reject — заглушки до Фазы 5.
  Без бэкенда остаются: settings (config-write), snapshots-каталог, testtree/testhistory (нужны Playwright-
  аннотации = категория B), reviewqueue (группировка диффов = C) — инертные пункты навигации до своих фаз.

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
