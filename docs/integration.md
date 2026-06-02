# Интеграция Pixela с репозиториями (где прогоняются тесты)

> Как репозиторий с Playwright-тестами подключается к Pixela: полный поток данных, что и куда
> отправляется, как резолвится baseline, что происходит при approve. **Фаза 5 завершена — петля замкнута**
> (см. §11). Технические идентификаторы (эндпоинты, env, поля) — на английском; объяснения — на русском.
>
> Связанные документы: архитектура бэкенда — [`architecture/go-backend.md`](architecture/go-backend.md);
> инварианты продукта — корневой [`../CLAUDE.md`](../CLAUDE.md); reporter —
> [`../packages/sdk/README.md`](../packages/sdk/README.md); контракт API — `spec/specs/04-api-contract.md`.

---

## 1. В двух словах

У тебя есть **обычный репозиторий с Playwright-тестами** (`expect(page).toHaveScreenshot()`). Pixela —
**self-hosted сервис ревью визуальных регрессий** поверх этих тестов. Интеграция = добавить один
custom-reporter в `playwright.config.ts` и прогонять тесты в CI как обычно. Reporter заливает скриншоты
в Pixela, Pixela показывает дифф и даёт workflow approve/reject, а «approve» (Mode A) готовит коммит
обновлённого baseline-PNG **обратно в твой репозиторий**.

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│  ТВОЙ РЕПОЗИТОРИЙ (например acme/storefront)                                            │
│                                                                                        │
│   tests/**.spec.ts            __screenshots__/**.png   ← baseline'ы живут в git (Mode A)│
│        │  expect(...).toHaveScreenshot()                     ▲                          │
│        ▼                                                     │ (approve → git commit/MR)│
│   Playwright run (CI: GitLab)                                │                          │
│        │  attachments: actual / expected / diff             │                          │
│        ▼                                                     │                          │
│   @pixela/playwright-reporter ───────────► POST /api/v1/...  │                          │
└────────────────────────────────────────────────┼───────────┼──────────────────────────┘
                                                  │           │
                              HTTPS, Authorization: ApiKey <key>
                                                  ▼           │
┌──────────────────────────────────────────────────────────────────────────────────────┐
│  PIXELA (self-hosted)                                                                   │
│                                                                                        │
│   serve  (ingestion API)  →  Postgres (метаданные)  →  River queue                     │
│        │                         ▲                          │                          │
│        ▼                         │                          ▼                          │
│   MinIO/S3 (PNG по sha256)       │                     worker (diff: pixelmatch)        │
│                                  │                          │                          │
│   web dashboard (Angular) ◄──────┴── presigned URL ◄────────┘                          │
│        │  review: 2-up / overlay / onion / шторка, approve / reject                    │
│        └──────────────────────────────────────────────────► GitLab MR status (Mode A) │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Модель: Mode A (git-native baseline)

**Эталон (baseline) живёт в твоём git-репозитории** — ровно там, где его хранит Playwright
(`snapshotPathTemplate`, по умолчанию рядом с тестом). Pixela — **слой ревью**, а не источник правды
о baseline. Это инвариант #1 продукта: Pixela **не** делает серверный merge-base resolution и **не**
владеет эталоном.

Что это значит на практике:

- **Сравнение «прошёл/не прошёл» делает сам Playwright** локально: `toHaveScreenshot` сверяет свежий
  скриншот с закоммиченным baseline-PNG и падает, если они разошлись.
- **Pixela получает результат на ревью**: reporter заливает _новый_ скриншот, _эталонный_ скриншот и
  (опционально) серверный дифф-оверлей. В дашборде ты решаешь — это баг или ожидаемое изменение.
- **Approve = подготовка git-коммита** обновлённого baseline-PNG в твоём репозитории (через ветку/MR),
  а не запись «нового эталона» в БД Pixela. Смержил MR → следующий прогон зелёный.

Альтернатива (Mode B, «Pixela владеет baseline») в v1 **не** реализуется.

---

## 3. Действующие лица

| Компонент | Где | Роль |
|---|---|---|
| Playwright + тесты | твой репо | прогоняет `toHaveScreenshot`, хранит baseline-PNG в git |
| `@pixela/playwright-reporter` | твой репо (devDep) | собирает скриншоты, заливает в Pixela по API-ключу |
| `pixela serve` | Pixela | ingestion API (приём сборок/снимков/картинок), dashboard API |
| `pixela worker` | Pixela | async diff (pixelmatch), финализация сборки |
| Postgres | Pixela | метаданные: проекты, сборки, снимки, baseline'ы, события |
| MinIO/S3 | Pixela | PNG, content-addressable по sha256 (дедуп) |
| Web dashboard | Pixela | ревью диффов, approve/reject, история |
| GitLab | внешний | источник CI-метаданных; адресат baseline-коммита + статуса MR (Mode A, опц.) |

---

## 4. Разовая настройка

### 4.1. На стороне Pixela (один раз на проект)

```bash
# поднять инфру + сервисы (детали — в корневом README)
pnpm dev:infra                 # postgres + redis + minio
pnpm migrate                   # схема + River-таблицы
pixela serve                   # ingestion + dashboard API на :3000
pixela worker                  # diff-воркер

# завести проект-репозиторий и выпустить API-ключ (показывается ОДИН раз)
pixela project create "Storefront" acme-storefront
pixela apikey create acme-storefront ci
#  → pxl_live_xxxxxxxxxxxxxxxx   (положить в CI-секреты)
```

Один **проект Pixela = один репозиторий**. Все данные изолированы по проекту: каждый запрос
аутентифицируется ключом проекта, данные проектов не пересекаются (инвариант #5).

Чтобы approve коммитил эталон обратно в git и репортил статус в MR — привяжи проект к GitLab-репо
(`pixela project set-gitlab` + `GITLAB_TOKEN`/`PUBLIC_URL`), см. §4.4. Без этого approve работает, но
git-write-back и MR-статус выключены (no-op).

### 4.2. В репозитории с тестами

```bash
pnpm add -D @pixela/playwright-reporter      # @playwright/test — peer-зависимость
```

> ⚠️ Сейчас пакет `private` и **не опубликован в npm-реестр** (см. §11). До публикации подключается
> локальным `npm pack` / `file:`-зависимостью / `pnpm link`. После публикации команда выше заработает как есть.

`playwright.config.ts`:

```ts
import { defineConfig } from '@playwright/test';

export default defineConfig({
  // baseline-снимки лежат в репозитории (Mode A, git-native)
  snapshotPathTemplate: '{testDir}/__screenshots__/{testFilePath}/{arg}-{projectName}{ext}',

  reporter: [
    ['list'],
    ['@pixela/playwright-reporter', {
      // apiUrl / projectKey лучше брать из env (ключ — не в конфиг!)
      // softMode: true (по умолчанию) — сбои API только варнят, тесты не валят
      // uploadBaseline: true (по умолчанию) — Mode A: грузить и эталон для ревью
    }],
  ],
});
```

Конфиг reporter'а (опция → env → CI-автодетект → git → дефолт):

| Опция | Env | По умолчанию |
|---|---|---|
| `apiUrl` | `PIXELA_URL` | — (обязателен) |
| `projectKey` | `PIXELA_API_KEY` | — (обязателен) |
| `project` | `PIXELA_PROJECT` | метаданные, опционально |
| `branch` | `CI_COMMIT_REF_NAME` | `git rev-parse --abbrev-ref HEAD` |
| `commit` | `CI_COMMIT_SHA` | `git rev-parse HEAD` |
| `ciBuildId` | `CI_PIPELINE_ID` | уникальный локальный id |
| `ciJobUrl` | `CI_JOB_URL` | — |
| `mrIid` | `CI_MERGE_REQUEST_IID` | — |
| `parallelTotal` | `CI_NODE_TOTAL` | `1` |
| `softMode` | — | `true` |
| `uploadBaseline` | — | `true` |
| `ignore` | — | `[]` (подстроки имён снимков для пропуска) |

### 4.3. В CI (`.gitlab-ci.yml`)

```yaml
visual-tests:
  stage: test
  image: mcr.microsoft.com/playwright:v1.49.0
  variables:
    PIXELA_URL: "https://pixela.internal"     # адрес твоего self-hosted Pixela
  script:
    - pnpm install
    - pnpm exec playwright test
  # PIXELA_API_KEY — в защищённых CI-переменных (Settings → CI/CD → Variables), не в .yml
```

Бранч/коммит/pipeline/MR/шарды Pixela **подхватывает из GitLab-CI окружения автоматически** —
руками прокидывать не нужно.

### 4.4. GitLab-проводка (Mode A write-back + MR-статус, опционально)

Чтобы approve **коммитил эталон обратно в репо** и Pixela репортил **commit status в MR**, привяжи проект
Pixela к его GitLab-репозиторию и дай сервису токен с правом push:

```bash
# 1) привязать проект Pixela к GitLab project ID (число из Settings → General, или path "group/repo")
pixela project set-gitlab acme-storefront 12345678
```

```bash
# 2) на процессах serve + worker — env с доступом к GitLab API:
GITLAB_BASE_URL=https://gitlab.com           # или твой self-hosted GitLab
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxx           # PAT/deploy-token: api + write_repository
PUBLIC_URL=https://pixela.internal            # внешний адрес дашборда — target-ссылка в commit-status
```

| Env | Назначение |
|---|---|
| `GITLAB_BASE_URL` | хост GitLab (Commits + Statuses API). Дефолт `https://gitlab.com` |
| `GITLAB_TOKEN` | токен с `api` + `write_repository`. **Пусто ⇒ git-write-back и MR-статус выключены (no-op)** |
| `PUBLIC_URL` | база для `target_url` в commit-status (кнопка ведёт на сборку в дашборде) |

**Что делает approve при настроенной проводке.** В одной транзакции с регистрацией эталона ставятся две
River-задачи (воркер `internal/gitsync`): `git_commit` пишет baseline-PNG из CAS обратно в ветку сборки
(`CHANGED→update`, `NEW→create`, `REMOVED→delete`, base64 через Commits API), `git_status` шлёт
`success/failed/pending` в commit MR (Statuses API) со ссылкой на дашборд. На финализации сборки начальный
вердикт зеркалится в MR автоматически (без кнопки).

**Если `set-gitlab` не задан или `GITLAB_TOKEN` пуст** — обе задачи тихо no-op'ятся: approve/reject и
запись `baselines` работают, но в git ничего не пишется и MR-статус не шлётся. Это штатный режим для
локального/без-GitLab использования.

---

## 5. Что происходит при каждом прогоне

### 5.1. Сбор снимков (reporter, фаза прогона)

Reporter слушает Playwright и на `onTestEnd` забирает аттачменты `toHaveScreenshot`:

- Playwright по умолчанию прикладывает `expected` / `actual` / `diff` **только когда снимок разошёлся**
  (или на первом прогоне, когда baseline ещё не зафиксирован). Совпавшие снимки аттачментов не дают.
- Reporter мёржит `actual` (новый) + `expected` (эталон) одного снимка по ключу
  `name :: browser :: viewport`, толерантен к ретраям.

`name` снимка — стабильный: путь из заголовков теста + проект Playwright + вьюпорт.

### 5.2. Контекст сборки (CI-автодетект)

Перед заливкой reporter резолвит `BuildContext`: `branch`, `commitSha`, `ciBuildId`
(ключ агрегации шардов), `ciJobUrl`, `mrIid`, `parallelTotal`. Приоритет: опции → GitLab-env → git → дефолт.

### 5.3. Загрузка в Pixela (`onEnd`, двухшаговый CAS-аплоад)

Все вызовы идут с заголовком `Authorization: ApiKey <key>` и проходят изоляцию по проекту.

1. **`POST /api/v1/builds`** — создать (или присоединиться к) сборку. Тело: `branch`, `commitSha`,
   `ciBuildId`, `ciJobUrl`, `mrIid`, `parallelTotal`. Сборка идемпотентна по `ciBuildId` — все шарды
   одного пайплайна сходятся в **одну** сборку.
2. На каждый снимок — **`POST /api/v1/builds/{buildId}/snapshots`**: декларация по хэшу
   (`name`, `browser`, `viewport`, `imageSha256`, `width`, `height`, `byteSize`). Ответ говорит,
   нужно ли заливать байты (`needUpload`) — если такой sha256 уже в сторадже, **дедуп**, заливать не надо.
3. Если нужно — **`PUT /api/v1/images/{sha256}`** с PNG-байтами. Сервер проверяет, что
   реальный sha256 совпадает с заявленным (`SNAPSHOT_HASH_MISMATCH` иначе), валидирует PNG (magic/размер).
   Хранение — content-addressable: один и тот же скриншот в N снимках = один объект в MinIO.
4. (Mode A) то же для эталонного PNG — best-effort: если упало, прогон не валится.

### 5.4. Финализация

**`PATCH /api/v1/builds/{buildId}`** с `{ "status": "FINALIZE" }` — фиксирует, что все снимки залиты:

- вычисляет **REMOVED** (baseline'ы, для которых в этой сборке нет снимка),
- переводит сборку в `COMPARING`,
- **транзакционно** ставит diff-задачи в очередь (River) — стейт и enqueue коммитятся вместе.

> Ingestion — **stateless**: только принимает и кладёт в очередь. Никакого синхронного diff в HTTP-запросе
> (инвариант #3).

### 5.5. Async diff (worker)

`pixela worker` берёт задачу на снимок и:

1. **Резолвит baseline строго по ветке** — `GetBaselineForKey(project, branch, name, browser, viewport)`.
   **Никакого merge-base** (инвариант #1).
2. Нет baseline → снимок **NEW**, diff не нужен.
3. Есть baseline → скачивает оба PNG, декодирует, гоняет `pixelmatch`, классифицирует:
   **UNCHANGED** (≤ порога) / **CHANGED** (выше). На CHANGED — кодирует diff-PNG, content-address по
   _декодированным_ пикселям (детерминизм, тег `pixela-diff/v1`), кладёт в MinIO.
4. Битый PNG → снимок **ERROR** (изоляция: одна плохая картинка не валит остальную сборку).
5. Когда обработан последний снимок — транзакционно ставится `FinalizeBuildJob`, который пересчитывает
   итог сборки: **PASSED** (всё чисто) / **REVIEW_REQUIRED** (есть изменения).

### 5.6. Review в дашборде

Сборки `REVIEW_REQUIRED` ждут человека. В review-воркспейсе (presigned-URL картинок из MinIO):
**рядом / наложение / onion / шторка**, синхронный зум (F-26), история по снимку, клавиши A/R.

### 5.7. Approve → git-native + статус в GitLab MR (✅ реализовано, Фаза 5)

Это сердцевина Mode A. Решение зафиксировано в [ADR 0002](adr/0002-baseline-ownership.md). Механика —
в §4.4: approve регистрирует эталон в `baselines` и **транзакционно** ставит две River-задачи — `git_commit`
(пишет baseline-PNG из CAS обратно в репо: CHANGED→update / NEW→create / REMOVED→delete) и `git_status`
(commit status в MR). Без настроенного `set-gitlab`/`GITLAB_TOKEN` git-задачи — тихий no-op.

**Почему фиче-ветку нельзя смержить до approve.** Ты на ветке, где UI изменился намеренно (например,
кнопка стала зелёной). В репо лежит **старый** baseline-PNG (синяя кнопка). Сам **Playwright** на прогоне
сравнивает новый скриншот со старым эталоном → `toHaveScreenshot` падает → **визуальный джоб красный**.
Reporter при этом в soft-mode джоб **не** валит — он только заливает дифф в Pixela. Branch-protection
требует зелёный визуальный джоб → **смержить нельзя**, пока эталон не обновлён.

> Важно: в репо не «нет эталона», а **устаревший** эталон. Роняет тест несовпадение «старый baseline ≠
> новый UI», а не отсутствие файла.

**Цикл approve → merge:**

```
1. фиче-ветка: UI изменился          → Playwright: toHaveScreenshot ❌ → визуальный джоб КРАСНЫЙ
2. reporter залил дифф в Pixela      → сборка REVIEW_REQUIRED
3. ты ревьюишь в дашборде            → APPROVE (новый вид правильный)
4. Pixela коммитит НОВЫЙ baseline-PNG в ЭТУ ЖЕ ветку  (push в ветку — атомарно: PR
   теперь содержит и код, и его эталон одним набором)
5. новый коммит триггерит пайплайн   → Playwright: новый UI vs новый эталон ✅ → джоб ЗЕЛЁНЫЙ
6. merge
```

`Reject` — наоборот: помечает изменение как нежелательное, baseline **не** трогается, визуальный джоб
остаётся красным (мержить нельзя, пока не починишь UI). Параллельно Pixela репортит **статус в GitLab MR**
(по `mrIid`): «N снимков ждут ревью» / «всё approved».

**Требование для шага 4.** Pixela нужен **git-доступ на запись** в репозиторий (deploy key / token с
правом push в ветку) — иначе он не сможет закоммитить эталон. Если прямой push в фиче-ветки не
приветствуется, альтернатива — Pixela открывает **отдельный маленький MR в твою ветку** (менее атомарно,
безопаснее по правам). Push в ту же ветку эргономичнее; см. ADR 0002.

---

## 6. Шардинг / параллельные джобы

Большие сьюты гоняются шардами (`playwright test --shard=i/N`, GitLab `parallel:`). Все шарды одного
пайплайна имеют общий `CI_PIPELINE_ID` → reporter шлёт один и тот же `ciBuildId` → Pixela **агрегирует их
в одну сборку** (идемпотентный create + `parallelTotal`). Финализация ждёт все шарды.

---

## 7. Аутентификация и изоляция

- Каждый прогон аутентифицируется **API-ключом проекта** (`Authorization: ApiKey pxl_...`). Ключ хранится
  как HMAC-хэш (не в открытом виде); показывается один раз при создании.
- Все ingestion-операции жёстко привязаны к проекту ключа — снимок нельзя залить в чужой проект (403).
- Dashboard — отдельная аутентификация: server-side сессии в Redis (cookie `pixela_session`), доступ к
  данным проекта только у его участников (membership). Reporter сессии **не** использует — только API-ключ.

---

## 8. Жизненный цикл одного baseline

| Шаг | В репозитории | В Pixela | Статус снимка |
|---|---|---|---|
| Первый прогон нового снимка | baseline-PNG ещё не закоммичен → Playwright создаёт actual | baseline не зарегистрирован | **NEW** |
| Approve первого | коммит нового baseline-PNG в репо (MR) | регистрируется baseline | — |
| Прогон без изменений | actual == baseline → Playwright не аттачит | (снимок не приходит / UNCHANGED) | **UNCHANGED** |
| Прогон с изменением | actual != baseline → Playwright падает, аттачит | diff-воркер: **CHANGED** + diff-оверлей | **CHANGED** |
| Approve изменения | коммит обновлённого baseline-PNG (MR) | baseline сдвигается на новый | — |
| Reject изменения | baseline не трогается | помечено rejected, тест остаётся красным | **REJECTED** |
| Снимок удалён из тестов | нет actual в сборке | finalize: **REMOVED** | **REMOVED** |

---

## 9. Хранение изображений

Картинки физически лежат в **двух** местах, и в обоих это настоящие PNG-файлы. Хэши (sha256) — это
**внутренний механизм адресации Pixela**, а не то, что хранится в репозитории.

| Где | Что физически лежит | Форма |
|---|---|---|
| **Тест-репо (git)** | baseline-эталоны | **полные PNG** (Mode A; рекомендуется Git LFS) |
| **Pixela → MinIO/S3** (твой сервер) | new + baseline + diff | **полные PNG, ключ = sha256** (дедуп) |
| **Pixela → Postgres** (твой сервер) | метаданные снимков/сборок | **хэши-указатели**, без байтов |

**1. Тест-репозиторий (git) — полные baseline-PNG, не хэши.** В Mode A эталоны живут в репо как обычные
бинарные файлы (`__screenshots__/**.png`), закоммиченные в git — так работает Playwright: `toHaveScreenshot`
сверяет свежий скриншот с **файлом** эталона. То есть в репо лежат **настоящие картинки**, а не их хэши.
Плата — бинарники пухнут историю; стандартное смягчение — **Git LFS** на каталог снимков (в pack идут
указатели, сами PNG — в LFS-сторадже).

**2. Pixela → MinIO/S3 — все картинки, content-addressable.** Pixela self-hosted, поэтому **всё на твоём
сервере**, ничего наружу. В объектном сторадже под ключом = sha256 лежат: `new` (свежий снимок), `baseline`
(копия эталона — Mode A грузит и его для ревью/истории) и `diff` (оверлей, который Pixela вычисляет сама).
Отсюда **дедуп**: один и тот же скриншот в N снимках/сборках = один объект. Reporter перед заливкой
спрашивает сервер по хэшу (`declareSnapshot` → `needUpload`) — если sha256 уже есть, байты не перезаливаются.

**3. Pixela → Postgres — только метаданные + хэши-указатели.** Байты картинок в Postgres **никогда** не
лежат (жёсткий инвариант). Таблица `images`: `sha256 (PK) · width · height · byte_size · created_at`; снимок
ссылается на блобы по хэшу (`new_image_sha`, `diff_image_sha`, baseline через `baseline_id`). Дашборд отдаёт
картинку через **presigned-URL** прямо из MinIO — фронт в стораж напрямую не ходит.

**Поток одной картинки:** Playwright делает скриншот → reporter считает sha256 → `declareSnapshot` шлёт хэш
→ сервер отвечает, есть ли блоб → если нет, `PUT /images/{sha256}` заливает байты в MinIO + строка
метаданных в Postgres.

> **Retention / рост (планируется, Фаза 7 деплой).** На сервере `new`/`diff` копятся по каждой сборке;
> `baseline` дедуплицируются. Разумная политика: чистить `new`/`diff` старше N дней/сборок, эталоны и
> approved-историю держать дольше; GC блобов, на которые не ссылается ни одна строка. Пока не реализовано —
> закладывается в фазу деплоя.

---

## 10. Где живёт эталон: git (Mode A) vs сервер (Mode B)

> Формально это решение зафиксировано в [ADR 0002 — Baseline ownership](adr/0002-baseline-ownership.md).
> Ниже — развёрнутое объяснение развилки.

Резонный вопрос: не логичнее ли хранить эталон **только** на сервере Pixela и тянуть его в рантайме, а репо
держать чистым? Это переход от **Mode A** (git владеет эталоном — текущий инвариант #1) к **Mode B**
(сервер владеет эталоном). Развилка настоящая; разберём честно.

**Сначала важное уточнение:** Pixela **уже** хранит всю историю (new + baseline + diff + события) на сервере
— независимо от Mode A/B. Так что «вся история на сервере» уже выполнено. Вопрос **узкий**: где живёт
именно **эталон-для-сравнения** (гейт «прошёл/не прошёл»).

| | **Mode A — git владеет эталоном** (текущий) | **Mode B — сервер владеет эталоном** |
|---|---|---|
| Эталон в репо | да (PNG/LFS) | нет, тянется из Pixela в рантайме |
| Размер репо | растёт (решается Git LFS) | минимальный |
| Approve | git-коммит/MR (медленнее, но в ревью кода) | API-операция (мгновенно) |
| Атомарность | PR меняет UI **и** эталон одним коммитом | эталон в БД отдельно от кода → риск дрейфа |
| Детерминизм прогона | герметично, без сети; baseline = что в дереве | зависит от сети + резолва «какой baseline?» |
| «Какой эталон брать?» | тривиально: тот, что в дереве на этом коммите | нужен резолв по ветке/merge-base (сложно, флак) |
| Откат | revert коммита откатывает и эталон | revert кода **не** откатывает эталон в БД |
| Pixela down во время CI | тесты идут как обычно (Pixela — слой ревью) | прогон ломается/флачит |

**Почему проект выбрал Mode A (инвариант #1).** Mode B возвращает ровно те проблемы, ради которых выбрали A:
- **резолв «какой baseline»** в рантайме (по ветке? merge-base с main?) — это и есть запрещённый инвариантом
  серверный merge-base resolution, главный источник «инструмент иногда врёт»;
- **сетевая связанность** прогона с Pixela — нарушает «детерминизм важнее фич» и герметичность тестов;
- **потеря атомарности** — изменение UI и его эталона разъезжаются (код в git, эталон в БД Pixela) → дрейф;
- git **уже** решает версионирование/ветки/мерж/откат эталонов — в Mode B это переизобретается в БД сервера
  (этим живут Percy/Chromatic/Argos — ценой большого слоя baseline-resolution).

**Рекомендация — гибрид, который даёт почти всё, что ты хочешь:** **Mode A + Git LFS**.
- рабочее дерево чистое (PNG лежат в LFS, не пухнут git-pack — «в репо только указатели»);
- детерминизм и атомарность сохранены (эталон версионируется с кодом);
- **вся история — всё равно на сервере Pixela** (new/baseline/diff/события централизованно).

То есть «хранить и там, и там» — но цена «там» (в репо) ≈ нулевая благодаря LFS, а выгоды Mode A остаются.
Если же чистота репо и мгновенный approve важнее детерминизма/герметичности — это **сознательная смена
инварианта #1** на Mode B; делается, но с явным «go» и пониманием, что придётся строить надёжный
baseline-resolution слой (и тесты станут зависеть от Pixela).

---

## 11. Что работает СЕГОДНЯ

По состоянию кода — **петля замкнута (Фаза 5 завершена)**:

| Возможность | Статус |
|---|---|
| Reporter: сбор скриншотов, CI-автодетект, шардинг, soft-mode, дедуп-аплоад | ✅ готово |
| Ingestion: create/declare/upload/finalize, изоляция по ключу, REMOVED, enqueue | ✅ готово |
| Async diff: pixelmatch, классификация UNCHANGED/CHANGED/NEW/ERROR, diff-PNG, детерминизм | ✅ готово |
| Dashboard: проекты/сборки/детали/review (4 режима, синхро-зум), members/baselines/activity | ✅ готово |
| **Регистрация baseline в Pixela** (`baselines` пишется при approve) | ✅ готово (Фаза 5) |
| **Approve / Reject ручки** в бэке (`/v1/{snapshots,builds}/{id}/{approve,reject}`) | ✅ готово (Фаза 5) |
| **Approve → git-коммит baseline-PNG обратно в репо** (суть Mode A) | ✅ готово (Фаза 5) — при `set-gitlab` |
| **Статус в GitLab MR** (commit status на финализации + по кнопке) | ✅ готово (Фаза 5) — при `set-gitlab` |

**Практическое следствие.** Регрессионная петля замкнута внутри Pixela: approve регистрирует эталон
(NEW→`baselines`), поэтому следующая сборка уже ловит **CHANGED** с серверным diff-оверлеем. Reporter шлёт
`baselinePath` (где эталон лежит в git), и при настроенном `set-gitlab` approve **коммитит обновлённый
baseline-PNG обратно в репозиторий** и репортит статус в MR. Без `gitlab_project_id`/`GITLAB_TOKEN`
git-write-back и MR-статус — **тихий no-op** (approve/reject и регистрация эталона работают всё равно),
так что локально/без GitLab сервис полноценно работает как «ingestion + review + approve».

---

## 12. Замыкание петли — сделано

Обе ступени реализованы:

- **A. Авто/ручной baseline.** Approve на любой сборке регистрирует эталон в `baselines`; на следующих
  прогонах diff-воркер честно ловит **CHANGED** и рисует оверлей. Прогон на `main` так же фиксирует эталоны.
- **B. Полноценно (Фаза 5).** Ручки approve/reject + запись baseline + Mode A коммит baseline-PNG в репо
  + статус в GitLab MR + проводка кнопок в review — настоящий git-native workflow. См. §4.4 (GitLab-проводка).

---

## 13. Чек-лист интеграции (когда петля будет замкнута)

1. Поднять Pixela: `dev:infra` → `migrate` → `serve` + `worker`.
2. `pixela project create` + `pixela apikey create` → положить ключ в CI-секреты.
3. В репо: `add -D @pixela/playwright-reporter`, добавить reporter в `playwright.config.ts`,
   убедиться что baseline-PNG коммитятся (Mode A, `snapshotPathTemplate`).
4. В CI: задать `PIXELA_URL` (env) и `PIXELA_API_KEY` (секрет), гонять `playwright test` как обычно.
5. Первый прогон на `main` → зафиксировать baseline'ы (авто или approve).
6. На фиче-ветке/MR → дифф приезжает в дашборд → review → approve (коммит baseline в репо) / reject.
7. Смержить approve-MR → следующий прогон зелёный.
