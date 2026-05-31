# Playwright — Example Usage (как тестовый проект подключает Pixela)

> Справочник для пользователей Pixela: как подключить визуальное тестирование к своему Playwright-проекту.
> Реализация репортера — Agent 05. Этот файл должен соответствовать итоговому SDK.

## Минимальное подключение

`playwright.config.ts`:

```ts
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  // baseline-снапшоты живут в репозитории (режим A, git-native).
  snapshotPathTemplate: '{testDir}/__screenshots__/{testFilePath}/{arg}-{projectName}{ext}',

  reporter: [
    ['list'],
    ['@pixela/playwright-reporter', {
      apiUrl: process.env.PIXELA_API_URL,
      projectKey: process.env.PIXELA_PROJECT_KEY,   // masked CI/CD variable
      softMode: true,                                // различие ревьюится в дашборде, не валит CI
      uploadBaseline: true,                          // заливать baseline для красивого ревью
    }],
  ],

  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'firefox',  use: { ...devices['Desktop Firefox'] } },
  ],
});
```

## Тест выглядит как обычный Playwright-тест

```ts
import { test, expect } from '@playwright/test';

test('events list — desktop', async ({ page }) => {
  await page.goto('/events');
  await page.waitForLoadState('networkidle');
  // стандартный Playwright-снимок; имя снапшота = идентичность для baseline
  await expect(page).toHaveScreenshot('events-list--desktop.png');
});
```

> Pixela не меняет API теста. Репортер собирает снятые скриншоты и заливает их для ревью/истории.
> baseline-файл (`events-list--desktop-chromium.png`) лежит в git и наследуется ветками — отсюда git-native модель.

## Детерминизм (обязательно — иначе всё «флачит»)

Подключи общую фикстуру детерминизма (см. `playwright/fixtures-and-determinism.md`): фиксированные локаль/TZ,
замороженные часы, отключённые анимации, маски динамических областей. Без этого diff будет шуметь и доверие
к инструменту упадёт.

## Локальный workflow

```bash
# прогнать тесты локально (создаст/сравнит baseline в __screenshots__)
npx playwright test

# обновить baseline локально (стандартный Playwright)
npx playwright test --update-snapshots
```

## CI workflow (GitLab)

`.gitlab-ci.yml` (фрагмент):

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
    # PIXELA_PROJECT_KEY — masked CI/CD variable в настройках проекта
  artifacts:
    when: always
    paths: [playwright-report/]
```

- 4 шарда агрегируются в ОДИН билд в Pixela (по `CI_PIPELINE_ID`).
- branch/commit/MR определяются автоматически из GitLab CI env.
- После прогона: статус в MR (pixela/visual) + (опц.) комментарий-сводка + уведомление в Telegram/Slack.

## Approve workflow (после ревью в дашборде)

git-native (см. `specs/06-baseline-strategy.md`). MVP-вариант:

```bash
# скачать approved-снапшоты текущего билда в пути baseline и закоммитить
npx pixela pull-baseline --build <buildId>
git add __screenshots__ && git commit -m "chore: update visual baseline"
git push
```

Если включён commit-через-API — Pixela сам создаёт коммит/MR с обновлёнными PNG (не нужно вручную).

## Что важно помнить

- baseline = файлы в git → сравнение всегда в пределах ветки → merge/rebase разруливает git.
- неизменившиеся скриншоты не льют байты повторно (дедупликация по sha256).
- softMode по умолчанию: визуальные различия не ломают CI, а собираются на ревью.
