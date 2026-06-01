# Pixela — Design Reference (dashboard / `apps/web`)

> **Что это.** Хэндофф-бандл из Claude Design (claude.ai/design): интерактивные
> HTML/CSS/JS-прототипы дашборда Pixela + Storybook-дизайн-система. Это **референс
> для фронтенда (Фаза 4+)**, а не продакшн-код. Задача кодинг-агента — воссоздать
> экраны **пиксель-в-пиксель в Angular** (standalone + signals + CDK, см.
> `docs/spec/frontend/angular-decisions.md`), а не копировать внутреннюю структуру
> прототипов.
>
> **Порядок чтения:** `README.md` (инструкция бандла) → `chats/chat1.md` (весь
> диалог с дизайн-ассистентом — там «живёт» намерение) → `project/design-system.html`
> (источник истины для компонентов) → конкретные экраны. **Не рендерить в браузере и
> не снимать скриншоты** без явной просьбы — всё (размеры, цвета, правила раскладки)
> читается прямо из HTML/CSS.

## Состав бандла (`docs/design/`)

- `README.md` — инструкция от Claude Design (read-this-first).
- `chats/chat1.md` — транскрипт итераций (RU). Фиксирует решения и пойманные баги.
- `project/` — прототипы:
  - **Источник истины:** `design-system.html` + `ds-components.css` + `ds-registry.js`
    (27 компонентов, все состояния, панель «Разметка» с копированием).
  - **Токены:** `styles.css` (см. ниже). Каркас: `app-shell.css`,
    `project-shell.js`, `org-shell.js`.
  - **Холст:** `Pixela.html` / `design-canvas.jsx` — сборка всех артбордов.
  - **Экраны:** `login.html`, `projects.html`, `members.html`, `settings.html`,
    `activity.html`, `buildlist.html`, `builddetail.html`, `review.html`,
    `reviewqueue.html`, `snapshots.html`, `baselines.html`, `testtree.html`,
    `testhistory.html`.
  - `assets/` — мок baseline/new/diff PNG для Review UI (960×820 → 960×880, ~3.47%
    diff). Заменяются на реальные Playwright-снимки.
  - `screenshots/` — отрендеренные превью экранов (только для ориентира).

## Язык и арт-дирекшн (зафиксировано в чате)

- **UI-текст — на русском**, технические данные — на английском (`feat/checkout-redesign`,
  хэши коммитов и т.п.).
- **Тёмная тема** на `#0E0E10`; элевация — через светлоту поверхности, не тени;
  hairline-границы 1px; **один** приглушённый акцент — индиго `#6E8AFA`.
- Шрифты: **Geist Sans** (UI) + **Geist Mono** (хэши/числа, `tabular-nums`).
- Линейка: Linear / Vercel Geist / GitHub Primer.

## Токены (из `project/styles.css` — переносить в Angular как CSS-переменные)

| Группа | Переменные |
|---|---|
| База/элевация | `--bg #0E0E10` · `--surface-1 #16161A` · `--surface-2 #1E1E24` · `--surface-3 #26262E` (hover/active) · `--surface-inset #0B0B0D` |
| Границы | `--border rgba(255,255,255,.08)` · `--border-strong .14` · `--border-faint .05` |
| Текст | `--text-1 .92` · `--text-2 .60` · `--text-3 .40` |
| Акцент | `--accent #6E8AFA` · `--accent-2 #8AA0FB` · `--accent-bg rgba(110,138,250,.14)` · `--accent-fg #0B0B0D` |
| Семантика | `--ok #4FB58A` (passed/approved/unchanged) · `--changed #E0A53E` (changed/review) · `--new #56A8D6` (new/info) · `--removed #E06A52` (removed/rejected/error) · `--neutral .40` (+ соответствующие `*-bg`) |
| Радиусы | `--radius 8px` · `--radius-sm 6px` · `--radius-lg 12px` |
| Шрифты | `--font Geist` · `--mono Geist Mono` |

Базовый размер 14px / line-height 1.45. **Статус всегда = иконка + текст + цвет**
(никогда только цвет — доступность). Ключевые компоненты в `styles.css`/`ds-components.css`:
`.status--*`, `.chip--*`, `.btn`/`.btn--primary|ghost|ok|danger`, `.seg` (segmented),
`.iconbtn`, `kbd`.

## Карта экранов → фичи → фаза

| Прототип | Экран | Фичи (`05-features.md`) | Фаза |
|---|---|---|---|
| `login.html` | Вход (email + SSO) | F-35, F-43 | 4 |
| `projects.html` | Обзор проектов организации (health, спарклайны) | F-20, F-36 | 4 |
| `members.html` | Участники/мониторинг (роли, активность) | F-36, F-49 | 4 |
| `settings.html` | Настройки: общие, интеграции, **пороги сравнения**, уведомления, токены API, danger zone | F-11, F-34, F-45, F-47/48 | 1 (токены) / 2 / 5 / 6 |
| `buildlist.html` | Лента сборок CI (+ empty / skeleton) | F-21 | 4 |
| `builddetail.html` | Детали сборки: грид миниатюр, счётчики, «только изменённые», batch | F-22, F-23, F-30 | 4/5 |
| `review.html` | **Ядро** — Review workspace: 2-up / overlay / onion / шторка, синхро-зум, A/R + 1/2/3, история | F-24, F-25, F-26, F-28, F-29, F-33 | 4/5 |
| `reviewqueue.html` | Очередь проверок — группировка одинаковых диффов («review once») | F-30, F-51 | 5 |
| `snapshots.html` | Каталог снимков проекта (фильтры, нестабильность) | F-53, F-54 | 4 |
| `baselines.html` | Базовые линии по веткам (устаревшие >90д) | F-32 (git-native) | 5 |
| `testtree.html` | Дерево тестов `проект/браузер → spec → describe → test → снимок` | — (read-only из Playwright-аннотаций) | 4 |
| `testhistory.html` | История теста: тренд diff% за 28 прогонов, эволюция базы, лента событий | F-50, F-54, F-33 | 4 |
| `activity.html` | Лента активности организации | F-56 | 6 |
| `design-system.html` | Storybook: 27 компонентов × состояния + примеры кода | — (основа всего UI) | 4 (делать первой) |
| SSE live-обновление в `COMPARING` | (по всем экранам сборки) | F-44 | 4 |

## Инварианты при переносе в Angular (не нарушать)

- **Компоненты раньше страниц.** Сперва дизайн-система (`design-system.html` →
  Angular standalone-компоненты + CDK), потом экраны из них — это явно
  сформулированное пожелание владельца в чате.
- **Источник правды контракта — OpenAPI бэка** (`packages/shared` из `pixela openapi`).
  Фронт типобезопасно потребляет его (`openapi-fetch`); мок-данные прототипов —
  только для верстки.
- **Baseline — git-native** (инвариант #1): экран `baselines.html` и approve в
  `review.html` готовят коммит/MR с PNG, сервер НЕ резолвит merge-base.
- **Без тяжёлых UI-китов** (инвариант CLAUDE.md): только Angular CDK + свои
  компоненты поверх этих токенов; Material целиком не тащить.
- Прошивку навигации в кликабельный прототип (вариант №1 в чате) владелец оставил
  кодинг-агенту — делается уже в Angular, не воспроизводится из HTML.
</content>
</invoke>
