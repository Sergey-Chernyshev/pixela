/**
 * @pixela/shared — TypeScript bindings for the Pixela API contract.
 *
 * `api.ts` is GENERATED from apps/api/api/openapi.yaml (run `pnpm gen`); never hand-edit it. The Go
 * handler structs are the single source of truth, so these types cannot drift from the server. This
 * barrel re-exports the raw generated `paths`/`components`/`operations` and adds ergonomic aliases for
 * the request/response DTOs the dashboard and SDK consume.
 */

export type { paths, components, operations } from './api';
import type { components } from './api';

type Schemas = components['schemas'];

// ---- Dashboard (Phase 4) ----
export type User = Schemas['User'];
export type ProjectView = Schemas['ProjectView'];
export type Counts = Schemas['Counts'];
export type BuildListItem = Schemas['BuildListItem'];
export type BuildsPage = Schemas['BuildsPage'];
export type SnapshotBrief = Schemas['SnapshotBrief'];
export type BuildDetail = Schemas['BuildDetail'];
export type Images = Schemas['Images'];
export type ApprovalEntry = Schemas['ApprovalEntry'];
export type SnapshotReview = Schemas['SnapshotReview'];
export type LoginRequest = Schemas['LoginInputBody'];
export type LoginResponse = Schemas['LoginOutputBody'];
export type LogoutResponse = Schemas['LogoutOutputBody'];
export type ProjectList = Schemas['ListProjectsOutputBody'];
export type Member = Schemas['Member'];
export type MemberList = Schemas['ListMembersOutputBody'];
export type BaselineView = Schemas['BaselineView'];
export type BaselineList = Schemas['ListBaselinesOutputBody'];
export type ActivityEntry = Schemas['ActivityEntry'];
export type ActivityList = Schemas['ListActivityOutputBody'];

// ---- Ingestion (Phase 1) ----
export type CreateBuildRequest = Schemas['CreateBuildInputBody'];
export type CreateBuildResponse = Schemas['CreateBuildOutputBody'];
export type DeclareSnapshotRequest = Schemas['DeclareSnapshotInputBody'];
export type DeclareSnapshotResponse = Schemas['DeclareSnapshotOutputBody'];
export type FinalizeBuildRequest = Schemas['FinalizeBuildInputBody'];

// ---- Errors ----
export type ApiError = Schemas['ApiError'];
export type ApiErrorBody = Schemas['ApiErrorBody'];

// ---- Domain enums ----
// Mirror the database enums (03-data-model). The codegen emits these fields as `string` (the Go DTOs
// carry the enum as a string), so the closed sets are declared here as the canonical client contract.

export type BuildStatus =
  | 'RUNNING'
  | 'COMPARING'
  | 'PASSED'
  | 'REVIEW_REQUIRED'
  | 'REJECTED'
  | 'ERROR';

export type SnapshotStatus =
  | 'PENDING'
  | 'UNCHANGED'
  | 'CHANGED'
  | 'NEW'
  | 'REMOVED'
  | 'APPROVED'
  | 'REJECTED'
  | 'ERROR';

export type Role = 'OWNER' | 'MEMBER';

export type ApprovalAction = 'APPROVE' | 'REJECT';

export type ErrorCode =
  | 'VALIDATION_ERROR'
  | 'UNAUTHORIZED'
  | 'FORBIDDEN_PROJECT'
  | 'NOT_FOUND'
  | 'BUILD_NOT_FOUND'
  | 'SNAPSHOT_HASH_MISMATCH'
  | 'IMAGE_TOO_LARGE'
  | 'BUILD_ALREADY_FINALIZED'
  | 'INVALID_CREDENTIALS'
  | 'INTERNAL';
