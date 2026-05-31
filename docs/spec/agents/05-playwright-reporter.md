# Agent 05 — Playwright Reporter & SDK

**Фаза:** 3 · **Фичи:** F-15..F-19 · **Зависит от:** Agent 02 (ingestion API)

## Задача
Реализовать `@pixela/playwright-reporter` — кастомный Playwright reporter, который заливает скриншоты в
Pixela из CI одной строкой подключения. Это то, ради чего весь проект.

## Контекст (прочитать)
- `playwright/reporter-design.md`, `playwright/example-usage.md`, `playwright/fixtures-and-determinism.md`,
  `specs/04-api-contract.md` (endpoints приёма), `specs/06-baseline-strategy.md` (режим A).

## Что сделать
1. Пакет `packages/sdk` → `@pixela/playwright-reporter`. Реализует интерфейс Playwright `Reporter`
   (`onBegin`, `onTestEnd`, `onEnd`).
2. **Конфиг** (F-16): через опции репортера в `playwright.config.ts` и/или env: `apiUrl`, `projectKey`,
   `branch`, `commit`, `ciBuildId`, `softMode`, `threshold`.
3. **Автоопределение GitLab CI** (F-17): если env-переменные CI заданы — заполнить branch/commit/ciBuildId/
   ciJobUrl/mrIid/parallelTotal из `CI_COMMIT_REF_NAME`, `CI_COMMIT_SHA`, `CI_PIPELINE_ID`, `CI_JOB_URL`,
   `CI_MERGE_REQUEST_IID`, `CI_NODE_TOTAL`. Явный конфиг переопределяет авто.
4. **Сбор скриншотов**: из результатов тестов забрать снятые PNG (attachments/snapshot-файлы). Для каждого:
   вычислить sha256, метаданные (name, browser из проекта Playwright, viewport).
   - Режим A: дочитать baseline-файл Playwright и тоже залить (для ревью/истории в дашборде).
5. **Заливка**: создать билд (onBegin/лениво), двухшаговая заливка снапшотов (объявить sha → PUT если нужно),
   финализировать (onEnd). Уважать дедупликацию (не лить байты, если needUpload=false).
6. **Шардинг** (F-18): несколько шардов → один билд. Стабильный buildId по (ciBuildId) — первый шард создаёт,
   остальные присоединяются; финализация по достижению parallelTotal (координация на сервере, Agent 02).
7. **Soft mode** (F-19): при различии не помечать тест-прогон failed (различие ревьюится в дашборде); строгий
   режим — опционально фейлить.
8. Пример проекта в `examples/playwright-demo/` + сквозная проверка против локального Pixela.

## Acceptance criteria
- [ ] Подключение реально в 1-2 строки в `playwright.config.ts`.
- [ ] В GitLab CI метаданные определяются автоматически; локально берутся из конфига/env.
- [ ] Скриншоты с корректными name/browser/viewport/sha приезжают в Pixela.
- [ ] Дедупликация работает: неизменившиеся PNG не льются повторно (needUpload=false).
- [ ] Шарды (`--shard`) агрегируются в ОДИН билд; финализация срабатывает после всех шардов.
- [ ] Soft mode не валит прогон при визуальном различии.
- [ ] Пример проекта проходит сквозной сценарий против локального Pixela; doc подключения отражает реальность.
- [ ] Тесты SDK: парсинг CI env, вычисление sha, протокол заливки (с моками API), агрегация шардов.

## Не делать
- Не снимать скриншоты самим (это делает Playwright).
- Не реализовывать сравнение в репортере как источник истины (сервер — источник статуса; клиентский diff опционален, B-08).
