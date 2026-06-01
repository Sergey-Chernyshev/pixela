# @pixela/playwright-reporter

A [Playwright](https://playwright.dev) custom reporter that uploads `toHaveScreenshot` images to
a self-hosted [Pixela](../../README.md) instance for visual-regression review.

It uses the **git-native baseline** model (Mode A): your baseline PNGs live in your repo exactly
as Playwright stores them, and the reporter additionally uploads the **new**, **baseline**, and
(server-computed) diff images to Pixela so you get a nice review UI and history. Pixela is a
review layer — it is not the source of truth for the baseline.

## Install

```bash
pnpm add -D @pixela/playwright-reporter
# @playwright/test is a peer dependency (you already have it)
```

## Register the reporter

In `playwright.config.ts`, add it to the `reporter` array — that's the whole setup:

```ts
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  // baseline snapshots live in the repo (Mode A, git-native)
  snapshotPathTemplate: '{testDir}/__screenshots__/{testFilePath}/{arg}-{projectName}{ext}',

  reporter: [
    ['list'],
    [
      '@pixela/playwright-reporter',
      {
        softMode: true, // visual diffs are reviewed in the dashboard, they don't fail CI
        uploadBaseline: true, // also upload the committed baseline for richer review (default)
      },
    ],
  ],

  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'firefox', use: { ...devices['Desktop Firefox'] } },
  ],
});
```

Your tests stay plain Playwright:

```ts
import { test, expect } from '@playwright/test';

test('events list — desktop', async ({ page }) => {
  await page.goto('/events');
  await expect(page).toHaveScreenshot('events-list--desktop.png');
});
```

> Read `docs/spec/playwright/fixtures-and-determinism.md` and wire up the determinism fixture
> (frozen clock, disabled animations, fixed locale/TZ/viewport). Without it, diffs will be noisy.

## Configuration

Reporter options (all optional) and the env vars they fall back to:

| Option           | Env                    | Default                         |
| ---------------- | ---------------------- | ------------------------------- |
| `apiUrl`         | `PIXELA_URL`           | — (required; disabled if unset) |
| `projectKey`     | `PIXELA_API_KEY`       | — (required; disabled if unset) |
| `project`        | `PIXELA_PROJECT`       | — (metadata only)               |
| `branch`         | `CI_COMMIT_REF_NAME`   | local git → `unknown`           |
| `commit`         | `CI_COMMIT_SHA`        | local git → `unknown`           |
| `ciBuildId`      | `CI_PIPELINE_ID`       | unique local id                 |
| `ciJobUrl`       | `CI_JOB_URL`           | —                               |
| `mrIid`          | `CI_MERGE_REQUEST_IID` | —                               |
| `parallelTotal`  | `CI_NODE_TOTAL`        | `1`                             |
| `softMode`       | —                      | `true`                          |
| `uploadBaseline` | —                      | `true`                          |
| `ignore`         | —                      | `[]` (skip names containing…)   |

Keep `PIXELA_API_KEY` out of source — use a masked CI/CD variable.

Explicit options always override auto-detected values. The API key is sent as
`Authorization: ApiKey <key>`.

## CI auto-detection (GitLab)

When running in GitLab CI the reporter reads `GITLAB_CI` + the `CI_*` variables above to populate
build metadata automatically — branch, commit, pipeline, MR IID, and shard count. Locally it
falls back to `git rev-parse` and a **unique** `ciBuildId` so local screenshots never aggregate
into someone else's CI build.

## Sharding

Run sharded as usual:

```yaml
visual-tests:
  image: mcr.microsoft.com/playwright:v1.49.0-jammy
  parallel: 4
  script:
    - npm ci
    - npx playwright test --shard=$CI_NODE_INDEX/$CI_NODE_TOTAL
  variables:
    PIXELA_URL: 'https://pixela.example.com'
    # PIXELA_API_KEY — masked CI/CD variable
```

The build is keyed by `ciBuildId` (`CI_PIPELINE_ID`), so every shard's reporter targets the
**same logical build**. The Pixela server upserts/aggregates and finalizes the build once all
`parallelTotal` shards have sent their `FINALIZE` (server-side, idempotent). Each shard's reporter
sends `parallelTotal` so the server knows how many to wait for.

## Two-phase upload & dedup

For each screenshot the reporter:

1. computes the `sha256` of the raw PNG bytes client-side and reads the PNG's width/height;
2. `POST /api/v1/builds/:id/snapshots` to declare it by hash → the server replies `needUpload`;
3. only when `needUpload` is `true`, `PUT /api/v1/images/:sha256` with the raw bytes.

Unchanged screenshots share a `sha256` with an existing blob, so their bytes are **not** uploaded
again — this is the content-addressable dedup that keeps CI traffic low.

## Soft mode & resilience

With `softMode: true` (default) any Pixela API error is logged as a warning and swallowed — a
visual upload failure never fails your test run. Transient errors (network, `429`, `5xx`) are
retried with exponential backoff inside the client first. Set `softMode: false` to make upload
failures propagate to the run's exit code.

If `PIXELA_URL` or `PIXELA_API_KEY` is missing, the reporter disables itself (logs a warning) and
the run proceeds normally.

## Public API

```ts
import PixelaReporter, {
  type PixelaReporterOptions,
  PixelaClient,
  resolveBuildContext,
  sha256Hex,
  parsePngDimensions,
} from '@pixela/playwright-reporter';
```

`PixelaReporter` is the default export (the Playwright reporter). The named exports are the
building blocks (HTTP client, CI detection, hashing, PNG parsing) for testing/advanced use.

## Scripts

```bash
pnpm run build      # tsc → dist/
pnpm run typecheck  # tsc --noEmit (includes tests)
pnpm test           # node:test via tsx
```
