import { readFile } from 'node:fs/promises';
import { basename } from 'node:path';

import type {
  FullConfig,
  FullResult,
  Reporter,
  Suite,
  TestCase,
  TestResult,
} from '@playwright/test/reporter';

import { resolveBuildContext, type BuildContext, type EnvLike } from './ci';
import { PixelaClient, PixelaApiError } from './client';
import { sha256Hex } from './hash';
import { parsePngDimensions } from './png';
import type { CreateBuildRequest } from './types';

/**
 * Reporter configuration. Every field is optional in `playwright.config.ts`; env vars provide
 * the rest. Explicit options always override auto-detected CI/git values.
 */
export interface PixelaReporterOptions {
  /** API base URL. Falls back to env PIXELA_URL. */
  apiUrl?: string;
  /** Project API key. Falls back to env PIXELA_API_KEY (preferred — keep keys out of config). */
  projectKey?: string;
  /** Optional logical project identifier (env PIXELA_PROJECT). Metadata only. */
  project?: string;
  /** Override branch (else CI_COMMIT_REF_NAME → git). */
  branch?: string;
  /** Override commit sha (else CI_COMMIT_SHA → git). */
  commit?: string;
  /** Override the shard-aggregation key (else CI_PIPELINE_ID → unique local id). */
  ciBuildId?: string;
  /** Override CI job URL (else CI_JOB_URL). */
  ciJobUrl?: string;
  /** Override MR IID (else CI_MERGE_REQUEST_IID). */
  mrIid?: string;
  /** Override expected shard count (else CI_NODE_TOTAL → 1). */
  parallelTotal?: number;
  /** When true (default), API failures only warn — they never fail the test run. */
  softMode?: boolean;
  /** Mode A: also read & upload the committed baseline PNG for review. Default true. */
  uploadBaseline?: boolean;
  /** Substrings of snapshot names to skip uploading. */
  ignore?: string[];
}

/** A screenshot to upload, collected during onTestEnd and flushed in onEnd. */
interface CollectedSnapshot {
  /** Stable snapshot name (test title path + project + viewport). */
  name: string;
  browser: string;
  viewport: string;
  /** Absolute path to the "new" (actual) PNG produced by Playwright, if a diff occurred. */
  actualPath?: string;
  /** Absolute path to the committed baseline PNG, if available (Mode A). */
  baselinePath?: string;
}

interface ResolvedConfig {
  apiUrl: string;
  apiKey: string;
  softMode: boolean;
  uploadBaseline: boolean;
  ignore: string[];
}

const LOG_PREFIX = '[pixela]';

/**
 * Pixela Playwright reporter (Mode A / git-native baseline).
 *
 * Lifecycle:
 *  - onBegin   → resolve build context (CI/git autodetect) and create the client.
 *  - onTestEnd → collect screenshot attachments (new + baseline) into an in-memory buffer.
 *  - onEnd     → two-phase upload each unique PNG (declare by sha → PUT bytes if needUpload),
 *                then PATCH the build to FINALIZE.
 *
 * Sharding: the build is keyed by ciBuildId (CI_PIPELINE_ID), so every shard's reporter
 * targets the same logical build. The server upserts/aggregates and finalizes once all
 * `parallelTotal` shards have sent their FINALIZE (idempotent on the server side).
 *
 * Soft mode: any Pixela API error is logged as a warning and swallowed — visual upload is not
 * a functional test and must never break the run. Transient errors are retried with backoff
 * inside the client first.
 */
export default class PixelaReporter implements Reporter {
  private readonly options: PixelaReporterOptions;
  private readonly env: EnvLike;
  private resolved: ResolvedConfig | undefined;
  private client: PixelaClient | undefined;
  private buildContext: BuildContext | undefined;
  /** Keyed by `${name}::${browser}::${viewport}` so actual+expected attachments merge. */
  private readonly snapshots = new Map<string, CollectedSnapshot>();
  /** Set when config is missing/disabled so we degrade to a no-op instead of crashing. */
  private disabledReason: string | undefined;

  constructor(options: PixelaReporterOptions = {}, env: EnvLike = process.env) {
    this.options = options;
    this.env = env;
  }

  printsToStdio(): boolean {
    return false;
  }

  onBegin(_config: FullConfig, _suite: Suite): void {
    const apiUrl = this.options.apiUrl ?? this.env.PIXELA_URL;
    const apiKey = this.options.projectKey ?? this.env.PIXELA_API_KEY;

    if (apiUrl === undefined || apiUrl.trim() === '') {
      this.disabledReason = 'PIXELA_URL / apiUrl is not set';
      this.warn(`disabled: ${this.disabledReason}`);
      return;
    }
    if (apiKey === undefined || apiKey.trim() === '') {
      this.disabledReason = 'PIXELA_API_KEY / projectKey is not set';
      this.warn(`disabled: ${this.disabledReason}`);
      return;
    }

    this.resolved = {
      apiUrl,
      apiKey,
      softMode: this.options.softMode ?? true,
      uploadBaseline: this.options.uploadBaseline ?? true,
      ignore: this.options.ignore ?? [],
    };

    this.buildContext = resolveBuildContext(
      {
        branch: this.options.branch,
        commit: this.options.commit,
        ciBuildId: this.options.ciBuildId,
        ciJobUrl: this.options.ciJobUrl,
        mrIid: this.options.mrIid,
        parallelTotal: this.options.parallelTotal,
      },
      this.env,
    );

    this.client = new PixelaClient({ baseUrl: apiUrl, apiKey });

    this.log(
      `build ${this.buildContext.ciBuildId} · branch ${this.buildContext.branch} · ` +
        `commit ${this.buildContext.commitSha.slice(0, 8)} · shards ${this.buildContext.parallelTotal}`,
    );
  }

  onTestEnd(test: TestCase, result: TestResult): void {
    if (this.disabledReason !== undefined || this.resolved === undefined) {
      return;
    }

    const browser = test.parent.project()?.name ?? 'unknown';
    const viewport = this.viewportOf(test);

    for (const attachment of result.attachments) {
      const collected = this.attachmentToSnapshot(test, attachment, browser, viewport);
      if (collected === undefined) {
        continue;
      }
      if (this.shouldIgnore(collected.name)) {
        continue;
      }
      const key = `${collected.name}::${browser}::${viewport}`;
      const existing = this.snapshots.get(key);
      if (existing === undefined) {
        this.snapshots.set(key, collected);
        continue;
      }
      // Merge actual + expected attachments of the same snapshot (and tolerate retries:
      // newest path wins per slot).
      this.snapshots.set(key, {
        name: existing.name,
        browser,
        viewport,
        actualPath: collected.actualPath ?? existing.actualPath,
        baselinePath: collected.baselinePath ?? existing.baselinePath,
      });
    }
  }

  async onEnd(_result: FullResult): Promise<void> {
    if (
      this.disabledReason !== undefined ||
      this.client === undefined ||
      this.buildContext === undefined
    ) {
      return;
    }

    try {
      const buildId = await this.ensureBuild();
      let uploaded = 0;
      let deduped = 0;

      for (const snap of this.snapshots.values()) {
        const result = await this.uploadSnapshot(buildId, snap);
        uploaded += result.uploaded;
        deduped += result.deduped;
      }

      await this.client.finalizeBuild(buildId);
      this.log(
        `finalized build ${buildId}: ${this.snapshots.size} snapshots ` +
          `(${uploaded} bytes-uploaded, ${deduped} deduped)`,
      );
    } catch (err) {
      this.handleError('upload/finalize failed', err);
    }
  }

  // --- internals -----------------------------------------------------------

  private async ensureBuild(): Promise<string> {
    if (this.client === undefined || this.buildContext === undefined) {
      throw new Error('client not initialized');
    }
    const ctx = this.buildContext;
    const body: CreateBuildRequest = {
      branch: ctx.branch,
      commitSha: ctx.commitSha,
      ciBuildId: ctx.ciBuildId,
      ...(ctx.ciJobUrl !== undefined ? { ciJobUrl: ctx.ciJobUrl } : {}),
      ...(ctx.mrIid !== undefined ? { mrIid: ctx.mrIid } : {}),
      parallelTotal: ctx.parallelTotal,
    };
    // The server upserts by ciBuildId, so every shard can safely POST: all join one build.
    const res = await this.client.createBuild(body);
    return res.buildId;
  }

  private async uploadSnapshot(
    buildId: string,
    snap: CollectedSnapshot,
  ): Promise<{ uploaded: number; deduped: number }> {
    if (this.client === undefined || this.resolved === undefined) {
      return { uploaded: 0, deduped: 0 };
    }
    let uploaded = 0;
    let deduped = 0;

    // Upload the "new" image when present (a diff occurred or first run produced an actual).
    if (snap.actualPath !== undefined) {
      const newRes = await this.uploadOne(
        buildId,
        snap.name,
        snap.browser,
        snap.viewport,
        snap.actualPath,
      );
      uploaded += newRes.uploaded;
      deduped += newRes.deduped;
    }

    // Mode A: best-effort upload of the committed baseline for review/history.
    if (this.resolved.uploadBaseline && snap.baselinePath !== undefined) {
      try {
        const baseRes = await this.uploadOne(
          buildId,
          snap.name,
          snap.browser,
          snap.viewport,
          snap.baselinePath,
        );
        uploaded += baseRes.uploaded;
        deduped += baseRes.deduped;
      } catch (err) {
        // Baseline upload is best-effort only — never let it break the run.
        this.warn(`baseline upload skipped for "${snap.name}": ${(err as Error).message}`);
      }
    }

    return { uploaded, deduped };
  }

  private async uploadOne(
    buildId: string,
    name: string,
    browser: string,
    viewport: string,
    path: string,
  ): Promise<{ uploaded: number; deduped: number }> {
    if (this.client === undefined) {
      return { uploaded: 0, deduped: 0 };
    }
    const bytes = await readFile(path);
    const sha256 = sha256Hex(bytes);
    const { width, height } = parsePngDimensions(bytes);

    const declared = await this.client.declareSnapshot(buildId, {
      name,
      browser,
      viewport,
      imageSha256: sha256,
      width,
      height,
      byteSize: bytes.byteLength,
    });

    if (declared.needUpload) {
      await this.client.uploadImage(sha256, bytes);
      return { uploaded: 1, deduped: 0 };
    }
    return { uploaded: 0, deduped: 1 };
  }

  /**
   * Map a Playwright attachment to a collected snapshot, or undefined if it isn't a visual
   * screenshot we care about. Playwright's toHaveScreenshot attaches PNGs named
   * `<snapshot>-actual.png` / `-expected.png` / `-diff.png` (and a bare `<snapshot>.png` on
   * first run). We treat "actual"/bare as the new image and "expected" as the baseline.
   */
  private attachmentToSnapshot(
    test: TestCase,
    attachment: TestResult['attachments'][number],
    browser: string,
    viewport: string,
  ): CollectedSnapshot | undefined {
    if (attachment.contentType !== 'image/png' || attachment.path === undefined) {
      return undefined;
    }
    const file = basename(attachment.path);
    if (file.endsWith('-diff.png')) {
      return undefined; // server recomputes diff; client diff is not the source of truth
    }

    const isExpected = file.endsWith('-expected.png');
    const isActual = file.endsWith('-actual.png');
    // A bare attachment that is NOT actual/expected/diff is a normal page screenshot, not a
    // visual-comparison snapshot — skip to avoid noise. (toHaveScreenshot always suffixes.)
    if (!isExpected && !isActual) {
      return undefined;
    }

    const name = this.snapshotName(test, file, browser, viewport);

    if (isExpected) {
      // Baseline-only attachment: upload it for review, no "new" image from this attachment.
      return {
        name,
        browser,
        viewport,
        baselinePath: attachment.path,
      };
    }

    // isActual: pair it with the committed baseline path if Playwright recorded one.
    return {
      name,
      browser,
      viewport,
      actualPath: attachment.path,
      baselinePath: this.expectedPathFor(test, attachment.path),
    };
  }

  /** Derive a stable snapshot name from the test title path + browser + viewport. */
  private snapshotName(test: TestCase, file: string, browser: string, viewport: string): string {
    // Prefer the attachment name (the arg passed to toHaveScreenshot), normalized.
    const base = file.replace(/-(actual|expected|diff)\.png$/, '').replace(/\.png$/, '');
    const titlePath = test
      .titlePath()
      .filter((p) => p.length > 0)
      .join(' › ');
    return `${titlePath} › ${base} [${browser} ${viewport}]`;
  }

  /** Given an "-actual.png" path, the sibling "-expected.png" is the committed baseline. */
  private expectedPathFor(_test: TestCase, actualPath: string): string | undefined {
    if (!actualPath.endsWith('-actual.png')) {
      return undefined;
    }
    return actualPath.replace(/-actual\.png$/, '-expected.png');
  }

  private viewportOf(test: TestCase): string {
    const use = test.parent.project()?.use as
      | { viewport?: { width: number; height: number } | null }
      | undefined;
    const vp = use?.viewport;
    if (vp && typeof vp.width === 'number' && typeof vp.height === 'number') {
      return `${vp.width}x${vp.height}`;
    }
    return 'unknown';
  }

  private shouldIgnore(name: string): boolean {
    const ignore = this.resolved?.ignore ?? [];
    return ignore.some((pattern) => name.includes(pattern));
  }

  private handleError(context: string, err: unknown): void {
    const message = err instanceof PixelaApiError ? `${err.message}` : (err as Error).message;
    if (this.resolved?.softMode ?? true) {
      this.warn(`${context}: ${message} (softMode — run not failed)`);
      return;
    }
    // Strict mode: surface the failure. Reporter errors propagate to the run's exit code.
    throw err instanceof Error ? err : new Error(`${context}: ${message}`);
  }

  private log(message: string): void {
    console.log(`${LOG_PREFIX} ${message}`);
  }

  private warn(message: string): void {
    console.warn(`${LOG_PREFIX} ${message}`);
  }
}
