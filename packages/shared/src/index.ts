/**
 * @pixela/shared — types mirroring docs/spec/specs/04-api-contract.md.
 *
 * Scaffold only. Concrete DTO/response types land from Phase 1 (ingestion) onward and are
 * kept in lockstep with the API contract so the dashboard can mirror them.
 */

/** Build lifecycle status (mirrors Prisma enum `BuildStatus`). */
export type BuildStatus =
  | 'RUNNING'
  | 'COMPARING'
  | 'PASSED'
  | 'REVIEW_REQUIRED'
  | 'REJECTED'
  | 'ERROR';

/** Per-snapshot comparison status (mirrors Prisma enum `SnapshotStatus`). */
export type SnapshotStatus =
  | 'PENDING'
  | 'UNCHANGED'
  | 'CHANGED'
  | 'NEW'
  | 'REMOVED'
  | 'APPROVED'
  | 'REJECTED'
  | 'ERROR';
