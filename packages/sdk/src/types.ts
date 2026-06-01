/**
 * Types mirroring the Pixela ingestion API contract (docs/spec/specs/04-api-contract.md).
 *
 * Kept self-contained in the SDK so the reporter has zero runtime/build coupling to other
 * workspace packages. These must stay in lockstep with the backend ingestion endpoints.
 */

/** Build lifecycle status returned by the ingestion endpoints. */
export type BuildStatus =
  | 'RUNNING'
  | 'COMPARING'
  | 'PASSED'
  | 'REVIEW_REQUIRED'
  | 'REJECTED'
  | 'ERROR';

/** `POST /api/v1/builds` request body. */
export interface CreateBuildRequest {
  branch: string;
  commitSha: string;
  ciBuildId: string;
  ciJobUrl?: string;
  /** Merge request IID, for MR status reporting. */
  mrIid?: string;
  /** Number of shards expected, for server-side aggregation/finalization. */
  parallelTotal?: number;
}

/** `POST /api/v1/builds` response body. */
export interface CreateBuildResponse {
  buildId: string;
  status: BuildStatus;
}

/** `POST /api/v1/builds/:buildId/snapshots` request body (phase 1: declare by hash). */
export interface DeclareSnapshotRequest {
  name: string;
  browser: string;
  /** Viewport as "WIDTHxHEIGHT", e.g. "1280x720". */
  viewport: string;
  /** sha256 of the raw PNG bytes, computed client-side (hex). */
  imageSha256: string;
  width: number;
  height: number;
  byteSize: number;
  /**
   * Repo-relative path of this snapshot's baseline file (Mode A / git-native). When set, approving the
   * snapshot in the dashboard commits the new image to this path on the build's branch. Optional.
   */
  baselinePath?: string;
}

/** `POST /api/v1/builds/:buildId/snapshots` response body. */
export interface DeclareSnapshotResponse {
  snapshotId: string;
  /** false when a blob with this sha already exists in storage (dedup hit). */
  needUpload: boolean;
}

/** `PATCH /api/v1/builds/:buildId` request body. */
export interface FinalizeBuildRequest {
  status: 'FINALIZE';
}

/** `PATCH /api/v1/builds/:buildId` response body. */
export interface FinalizeBuildResponse {
  buildId: string;
  status: BuildStatus;
}

/** Unified API error envelope (docs/spec/specs/04-api-contract.md §Ошибки). */
export interface ApiErrorEnvelope {
  error: {
    code: string;
    message: string;
  };
}
