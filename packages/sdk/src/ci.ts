import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { hostname } from 'node:os';

/**
 * Build context resolved from (in priority order) explicit reporter options → GitLab CI env →
 * local git → safe local fallbacks. This is what populates `POST /api/v1/builds`.
 */
export interface BuildContext {
  branch: string;
  commitSha: string;
  /**
   * Stable key that aggregates all shards of one CI pipeline into a single Pixela build.
   * In CI this is CI_PIPELINE_ID; locally it is a per-run unique value so local screenshots
   * never join someone else's build.
   */
  ciBuildId: string;
  ciJobUrl?: string;
  mrIid?: string;
  /** How many shards are expected (CI_NODE_TOTAL); defaults to 1. */
  parallelTotal: number;
}

/** Explicit overrides from reporter options. Any provided field wins over auto-detection. */
export interface BuildContextOverrides {
  branch?: string;
  commit?: string;
  ciBuildId?: string;
  ciJobUrl?: string;
  mrIid?: string;
  parallelTotal?: number;
}

/** Narrow record type for env so detection is unit-testable without touching process.env. */
export type EnvLike = Record<string, string | undefined>;

function nonEmpty(value: string | undefined): string | undefined {
  if (value === undefined) {
    return undefined;
  }
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : undefined;
}

/** True when running inside GitLab CI (the `CI` + `GITLAB_CI` markers are set). */
export function isGitlabCi(env: EnvLike): boolean {
  return nonEmpty(env.GITLAB_CI) !== undefined;
}

function gitOutput(args: readonly string[], cwd?: string): string | undefined {
  try {
    const out = execFileSync('git', args, {
      cwd,
      stdio: ['ignore', 'pipe', 'ignore'],
      encoding: 'utf8',
    });
    return nonEmpty(out);
  } catch {
    return undefined;
  }
}

function parsePositiveInt(value: string | undefined): number | undefined {
  if (value === undefined) {
    return undefined;
  }
  const n = Number.parseInt(value, 10);
  return Number.isInteger(n) && n > 0 ? n : undefined;
}

/**
 * Resolve the build context. Explicit overrides take precedence, then GitLab CI env vars,
 * then local git, then deterministic-but-unique local fallbacks.
 *
 * `gitRunner` is injectable so tests can avoid spawning git.
 */
export function resolveBuildContext(
  overrides: BuildContextOverrides = {},
  env: EnvLike = process.env,
  gitRunner: (args: readonly string[]) => string | undefined = gitOutput,
): BuildContext {
  const branch =
    nonEmpty(overrides.branch) ??
    nonEmpty(env.CI_COMMIT_REF_NAME) ??
    gitRunner(['rev-parse', '--abbrev-ref', 'HEAD']) ??
    'unknown';

  const commitSha =
    nonEmpty(overrides.commit) ??
    nonEmpty(env.CI_COMMIT_SHA) ??
    gitRunner(['rev-parse', 'HEAD']) ??
    'unknown';

  // Local runs MUST get a unique ciBuildId so they don't aggregate into a foreign build.
  const localBuildId = `local-${hostname()}-${Date.now()}-${randomBytes(4).toString('hex')}`;
  const ciBuildId = nonEmpty(overrides.ciBuildId) ?? nonEmpty(env.CI_PIPELINE_ID) ?? localBuildId;

  const ciJobUrl = nonEmpty(overrides.ciJobUrl) ?? nonEmpty(env.CI_JOB_URL);
  const mrIid = nonEmpty(overrides.mrIid) ?? nonEmpty(env.CI_MERGE_REQUEST_IID);

  const parallelTotal =
    overrides.parallelTotal ?? parsePositiveInt(nonEmpty(env.CI_NODE_TOTAL)) ?? 1;

  return { branch, commitSha, ciBuildId, ciJobUrl, mrIid, parallelTotal };
}
