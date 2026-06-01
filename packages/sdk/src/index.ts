/**
 * @pixela/playwright-reporter
 *
 * A Playwright custom reporter that uploads `toHaveScreenshot` images to a Pixela instance for
 * visual-regression review (Mode A / git-native baseline). Register it in the `reporter` array
 * of `playwright.config.ts`:
 *
 *   reporter: [
 *     ['list'],
 *     ['@pixela/playwright-reporter', { softMode: true }],
 *   ]
 *
 * Configuration is read from reporter options and env (PIXELA_URL, PIXELA_API_KEY,
 * PIXELA_PROJECT). CI metadata (branch/commit/pipeline/MR/shards) is auto-detected from GitLab
 * CI env vars, falling back to local git. See the package README for details.
 */
export { default, default as PixelaReporter } from './reporter';
export type { PixelaReporterOptions } from './reporter';

export { PixelaClient, PixelaApiError } from './client';
export type { ClientOptions } from './client';

export { resolveBuildContext, isGitlabCi } from './ci';
export type { BuildContext, BuildContextOverrides, EnvLike } from './ci';

export { sha256Hex } from './hash';
export { parsePngDimensions, isPng } from './png';
export type { PngDimensions } from './png';

export type {
  BuildStatus,
  CreateBuildRequest,
  CreateBuildResponse,
  DeclareSnapshotRequest,
  DeclareSnapshotResponse,
  FinalizeBuildRequest,
  FinalizeBuildResponse,
  ApiErrorEnvelope,
} from './types';

/** Package identifier (kept for backwards compatibility with the Phase-0 scaffold). */
export const PIXELA_REPORTER_PACKAGE = '@pixela/playwright-reporter';
