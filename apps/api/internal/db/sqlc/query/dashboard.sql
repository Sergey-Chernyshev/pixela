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
  b.id, b.branch, b.commit_sha, b.status, b.created_at, b.finalized_at, b.ci_job_url,
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
SELECT b.id, b.project_id, b.branch, b.commit_sha, b.status, b.created_at, b.finalized_at, b.ci_job_url
FROM builds b
JOIN memberships m ON m.project_id = b.project_id
WHERE b.id = @build_id AND m.user_id = @user_id;

-- The owning project of a build, regardless of membership — used to return a precise 403 (vs 404).
-- name: GetBuildProject :one
SELECT b.project_id FROM builds b WHERE b.id = @build_id;

-- Snapshots of a build (brief metadata + image shas for the build-detail thumbnail grid). The new and
-- diff blobs are the snapshot's own; the baseline blob is resolved through baseline_id. The service
-- presigns whichever are non-null so the grid can render a real thumbnail per card.
-- name: ListSnapshotsForBuild :many
SELECT s.id, s.name, s.browser, s.viewport, s.status, s.diff_ratio,
       s.new_image_sha, s.diff_image_sha, bl.image_sha AS baseline_image_sha
FROM snapshots s
LEFT JOIN baselines bl ON bl.id = s.baseline_id
WHERE s.build_id = @build_id
ORDER BY s.name, s.browser, s.viewport;

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

-- Membership-scoped project overview: each project the user belongs to, enriched with real aggregates
-- computed in SQL — open reviews (builds awaiting review), member count, last build time, and the latest
-- build's "in-norm" snapshot ratio (UNCHANGED+APPROVED over total) for the health bar. Every figure is
-- derived from existing rows; nothing is synthesised.
-- name: ListProjectsOverview :many
SELECT
  p.id, p.name, p.slug, p.default_branch, p.created_at, m.role,
  (SELECT count(*) FROM builds b WHERE b.project_id = p.id AND b.status = 'REVIEW_REQUIRED') AS open_reviews,
  (SELECT count(*) FROM memberships mm WHERE mm.project_id = p.id) AS member_count,
  lb.created_at AS last_build_at,
  lb.status     AS last_build_status,
  coalesce(lc.ok, 0)    AS health_ok,
  coalesce(lc.total, 0) AS health_total
FROM projects p
JOIN memberships m ON m.project_id = p.id
LEFT JOIN LATERAL (
  SELECT b.id, b.created_at, b.status
  FROM builds b WHERE b.project_id = p.id
  ORDER BY b.created_at DESC LIMIT 1
) lb ON true
LEFT JOIN LATERAL (
  SELECT
    count(*) FILTER (WHERE s.status IN ('UNCHANGED', 'APPROVED')) AS ok,
    count(*) AS total
  FROM snapshots s WHERE s.build_id = lb.id
) lc ON true
WHERE m.user_id = @user_id
ORDER BY p.created_at DESC;

-- Members of a project (membership-scoped at the handler): each member's identity, role, and a real
-- all-time review tally (approval events authored). Used by the Участники screen.
-- name: ListProjectMembers :many
SELECT
  u.id, u.email, u.name, m.role,
  (SELECT count(*) FROM approval_events ae WHERE ae.user_id = u.id) AS total_reviews
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.project_id = @project_id
ORDER BY m.role, u.email;

-- Per-branch baselines of a project (Базовые линии screen): the canonical accepted snapshot for each
-- (branch, name, browser, viewport), with the blob sha (presigned by the service), who accepted it, and
-- when it was last updated. Membership is enforced at the handler.
-- name: ListProjectBaselines :many
SELECT
  bl.id, bl.branch, bl.name, bl.browser, bl.viewport, bl.image_sha,
  bl.updated_at, bl.created_at,
  u.email AS approved_by_email
FROM baselines bl
LEFT JOIN users u ON u.id = bl.approved_by_user_id
WHERE bl.project_id = @project_id
ORDER BY bl.branch, bl.name, bl.browser, bl.viewport;

-- Organization activity feed: approval events across every project the user is a member of, newest
-- first. The membership EXISTS predicate keeps the feed scoped — a user never sees another org's events.
-- name: ListActivityForUser :many
SELECT
  ae.id, ae.action, ae.created_at,
  u.email      AS user_email,
  s.id         AS snapshot_id,
  s.name       AS snapshot_name,
  b.branch     AS branch,
  p.id         AS project_id,
  p.name       AS project_name
FROM approval_events ae
JOIN users u ON u.id = ae.user_id
JOIN snapshots s ON s.id = ae.snapshot_id
JOIN builds b ON b.id = s.build_id
JOIN projects p ON p.id = b.project_id
WHERE EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = @user_id AND m.project_id = p.id)
ORDER BY ae.created_at DESC
LIMIT @page_limit;
