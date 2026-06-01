-- Dashboard reads + user/membership management (Phase 4). All multi-project reads are scoped at the
-- query level to the requesting user's memberships (spec §10 F-36: WHERE project_id IN user's projects),
-- never by post-filtering in Go.

-- name: CreateUser :one
INSERT INTO users (id, email, name, password_hash)
VALUES (@id, @email, @name, @password_hash)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = @email;

-- name: GetUserByID :one
SELECT id, email, name FROM users WHERE id = @id;

-- name: CreateMembership :one
INSERT INTO memberships (id, user_id, project_id, role)
VALUES (@id, @user_id, @project_id, @role)
ON CONFLICT (user_id, project_id) DO UPDATE SET role = EXCLUDED.role
RETURNING *;

-- Projects the user belongs to (membership-scoped list).
-- name: ListProjectsForUser :many
SELECT p.id, p.name, p.slug, p.default_branch, p.created_at, m.role
FROM projects p
JOIN memberships m ON m.project_id = p.id
WHERE m.user_id = @user_id
ORDER BY p.created_at DESC;

-- Membership guard: is the user a member of this project? (used by per-resource 403 checks).
-- name: IsProjectMember :one
SELECT EXISTS (
  SELECT 1 FROM memberships WHERE user_id = @user_id AND project_id = @project_id
) AS is_member;

-- Builds feed for a project the user can see, with per-status snapshot counts computed in SQL
-- (count(*) FILTER), filtered by optional branch/status, paginated. The membership predicate is part of
-- the query so a non-member can never read another project's builds.
-- name: ListBuildsForProjectMember :many
SELECT
  b.id, b.branch, b.commit_sha, b.status, b.created_at, b.ci_job_url,
  count(s.id) FILTER (WHERE s.status = 'UNCHANGED') AS unchanged,
  count(s.id) FILTER (WHERE s.status = 'CHANGED')   AS changed,
  count(s.id) FILTER (WHERE s.status = 'NEW')       AS new,
  count(s.id) FILTER (WHERE s.status = 'REMOVED')   AS removed
FROM builds b
LEFT JOIN snapshots s ON s.build_id = b.id
WHERE b.project_id = @project_id
  AND EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = @user_id AND m.project_id = b.project_id)
  AND (@branch::text = '' OR b.branch = @branch)
  AND (@status::text = '' OR b.status::text = @status)
GROUP BY b.id
ORDER BY b.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- Total matching builds (for totalPages). Mirrors the filters/membership of the feed query.
-- name: CountBuildsForProjectMember :one
SELECT count(*)
FROM builds b
WHERE b.project_id = @project_id
  AND EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = @user_id AND m.project_id = b.project_id)
  AND (@branch::text = '' OR b.branch = @branch)
  AND (@status::text = '' OR b.status::text = @status);

-- A build the user may see (membership-scoped join). No row ⇒ not found OR not a member: the handler
-- distinguishes 404 vs 403 by also checking build existence/ownership.
-- name: GetBuildForMember :one
SELECT b.id, b.project_id, b.branch, b.commit_sha, b.status, b.created_at, b.ci_job_url
FROM builds b
JOIN memberships m ON m.project_id = b.project_id
WHERE b.id = @build_id AND m.user_id = @user_id;

-- The owning project of a build, regardless of membership — used to return a precise 403 (vs 404).
-- name: GetBuildProject :one
SELECT b.project_id FROM builds b WHERE b.id = @build_id;

-- Snapshots of a build (brief metadata for the build detail view).
-- name: ListSnapshotsForBuild :many
SELECT id, name, browser, viewport, status, diff_ratio
FROM snapshots
WHERE build_id = @build_id
ORDER BY name, browser, viewport;

-- A snapshot the user may see, with everything needed for the review view, scoped by membership.
-- name: GetSnapshotForMember :one
SELECT
  s.id, s.name, s.browser, s.viewport, s.status, s.diff_ratio, s.diff_pixels,
  s.new_image_sha, s.diff_image_sha, s.baseline_id,
  bl.image_sha AS baseline_image_sha,
  b.project_id
FROM snapshots s
JOIN builds b ON b.id = s.build_id
JOIN memberships m ON m.project_id = b.project_id
LEFT JOIN baselines bl ON bl.id = s.baseline_id
WHERE s.id = @snapshot_id AND m.user_id = @user_id;

-- The owning project of a snapshot, regardless of membership — used for a precise 403 (vs 404).
-- name: GetSnapshotProject :one
SELECT b.project_id
FROM snapshots s
JOIN builds b ON b.id = s.build_id
WHERE s.id = @snapshot_id;

-- Approval history feed for a snapshot (audit, spec §10): action / who / when.
-- name: ListApprovalEvents :many
SELECT ae.action, u.email AS user_email, ae.created_at
FROM approval_events ae
JOIN users u ON u.id = ae.user_id
WHERE ae.snapshot_id = @snapshot_id
ORDER BY ae.created_at DESC;
