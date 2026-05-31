# Playwright Reporter Design (`@pixela/playwright-reporter`)

> Детальный дизайн кастомного репортера. Реализация — Agent 05. Этот файл — справочник по интерфейсу и поведению.

## Зачем кастомный reporter, а не плагин/фикстура

Playwright Reporter API даёт хуки на жизненный цикл прогона (`onBegin`, `onTestEnd`, `onEnd`) и доступ к
результатам тестов и их attachments (включая снятые скриншоты). Это правильная точка интеграции: репортер не
вмешивается в сами тесты, работает поверх стандартного `toHaveScreenshot()` (режим A baseline), и собирает
всё в конце. Тесты остаются чистыми — подключение в `playwright.config.ts`.

## Интерфейс (Playwright Reporter)

```ts
import type { Reporter, FullConfig, Suite, TestCase, TestResult, FullResult } from '@playwright/test/reporter';

export interface PixelaReporterOptions {
  apiUrl: string;                 // https://pixela.example.com
  projectKey: string;            // ApiKey проекта (лучше из env PIXELA_PROJECT_KEY)
  branch?: string;               // override; иначе из CI env
  commit?: string;               // override; иначе из CI env
  ciBuildId?: string;            // override; иначе CI_PIPELINE_ID
  softMode?: boolean;            // default true: различие не валит прогон
  pixelThreshold?: number;       // прокидывается как метаданные/настройка проекта
  uploadBaseline?: boolean;      // режим A: дочитывать и заливать baseline-файлы (default true)
}

class PixelaReporter implements Reporter {
  onBegin(config: FullConfig, suite: Suite): void { /* создать/присоединиться к билду */ }
  onTestEnd(test: TestCase, result: TestResult): void { /* собрать скриншоты теста */ }
  async onEnd(result: FullResult): Promise<void> { /* залить всё + финализировать билд */ }
}
export default PixelaReporter;
```

## Поведение по хукам

**onBegin**
- Резолв контекста: branch/commit/ciBuildId/ciJobUrl/mrIid/parallelTotal (явный конфиг → иначе CI env, см. ниже).
- Определить buildId: стабильно по `ciBuildId` (чтобы шарды присоединились к одному билду). Первый шард
  создаёт билд (`POST /builds`), остальные используют тот же buildId (сервер апсертит/присоединяет).

**onTestEnd**
- Из `result.attachments` и snapshot-результатов вытащить PNG, относящиеся к visual-сравнению.
- Для каждого: name (стабильное имя снапшота из теста/файла), browser (из projectName/конфига), viewport.
- Буферизовать в памяти/на диске до onEnd (не лить по одному, чтобы батчить и дедупить).

**onEnd**
- Для каждого собранного скриншота: посчитать sha256; шаг 1 заливки (`POST /builds/:id/snapshots`,
  needUpload?), при needUpload — PUT байтов. В режиме A (uploadBaseline) — то же для baseline-файла.
- Финализировать билд (`PATCH /builds/:id` FINALIZE). При шардинге финализация на сервере срабатывает по
  достижении parallelTotal (репортер каждого шарда шлёт свою финализацию; сервер идемпотентно ждёт всех).
- softMode: вернуть управление, не влияя на exit code из-за визуальных различий. (Строгий режим — опц.)

## Автоопределение GitLab CI (env)

| Поле | Источник | Фолбэк |
|------|----------|--------|
| branch | `CI_COMMIT_REF_NAME` | git rev-parse (локально) |
| commit | `CI_COMMIT_SHA` | git rev-parse HEAD |
| ciBuildId | `CI_PIPELINE_ID` | timestamp+host (локально, чтобы не агрегировать чужое) |
| ciJobUrl | `CI_JOB_URL` | — |
| mrIid | `CI_MERGE_REQUEST_IID` | — |
| parallelTotal | `CI_NODE_TOTAL` | 1 |
| projectKey | `PIXELA_PROJECT_KEY` | опция репортера |

Явный конфиг всегда переопределяет авто. Локальный прогон без CI env должен иметь УНИКАЛЬНЫЙ ciBuildId,
чтобы локальные скриншоты не присоединялись к чужому билду.

## Дедупликация на клиенте

sha256 каждого PNG считается локально. Если сервер вернул needUpload=false (блоб уже есть — типично для
неизменившихся снимков, чей sha совпадает с baseline) — байты не отправляются. Это резко снижает CI-трафик.

## Ошибки и устойчивость

- Сетевые сбои заливки: ретраи с backoff; при исчерпании — внятная ошибка в лог репортера, но (в softMode)
  не валить прогон тестов жёстко (визуальная заливка — не функциональный тест).
- Таймаут финализации шарда: сервер не должен «вечно ждать» недоехавший шард (см. brainstorm B-09 —
  таймаут/политика на стороне сервера).

## Открытые вопросы (см. `prompts/02-brainstorm-prompts.md`)
- B-08: считать ли diff на клиенте для быстрого фейла, или только сервер — источник статуса.
- B-09: надёжная детекция «все шарды пришли», поведение при упавшем шарде.
