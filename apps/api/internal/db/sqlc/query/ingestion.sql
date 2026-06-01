-- Ingestion: builds, snapshots, images, baselines (Phase 1).

-- name: CreateBuild :one
INSERT INTO builds (id, project_id, branch, commit_sha, ci_build_id, ci_job_url, mr_iid, status)
VALUES (@id, @project_id, @branch, @commit_sha, @ci_build_id, @ci_job_url, @mr_iid, 'RUNNING')
RETURNING *;

-- name: GetBuild :one
SELECT * FROM builds WHERE id = @id;

-- Serialize concurrent finalize/recompute on a build (rulebook §8.3).
-- name: GetBuildForUpdate :one
SELECT * FROM builds WHERE id = @id FOR UPDATE;

-- name: SetBuildComparing :exec
UPDATE builds SET status = 'COMPARING', finalized_at = now() WHERE id = @id;

-- Image metadata is content-addressed; the blob bytes live in object storage. ON CONFLICT DO NOTHING
-- makes repeated declarations idempotent (the FK target for snapshots.new_image_sha).
-- name: UpsertImage :exec
INSERT INTO images (sha256, width, height, byte_size)
VALUES (@sha256, @width, @height, @byte_size)
ON CONFLICT (sha256) DO NOTHING;

-- name: ImageExists :one
SELECT EXISTS (SELECT 1 FROM images WHERE sha256 = @sha256) AS exists;

-- Idempotent snapshot declare (CI-retry safe) on the composite identity key. baseline_path is the
-- repo-relative path of this snapshot's baseline file (Mode A), refreshed on re-declare.
-- name: UpsertSnapshot :one
INSERT INTO snapshots (id, build_id, name, browser, viewport, new_image_sha, baseline_path, status)
VALUES (@id, @build_id, @name, @browser, @viewport, @new_image_sha, @baseline_path, 'PENDING')
ON CONFLICT (build_id, name, browser, viewport)
DO UPDATE SET new_image_sha = EXCLUDED.new_image_sha, baseline_path = EXCLUDED.baseline_path, status = 'PENDING'
RETURNING *;

-- Snapshots that still need a diff comparison (enqueued on finalize).
-- name: ListPendingSnapshotIDs :many
SELECT id FROM snapshots WHERE build_id = @build_id AND status = 'PENDING';

-- REMOVED detection: baselines of (project, branch) with no matching snapshot in this build.
-- name: ListMissingBaselines :many
SELECT b.id, b.name, b.browser, b.viewport
FROM baselines b
WHERE b.project_id = @project_id AND b.branch = @branch
  AND NOT EXISTS (
    SELECT 1 FROM snapshots s
    WHERE s.build_id = @build_id
      AND s.name = b.name AND s.browser = b.browser AND s.viewport = b.viewport
  );

-- name: InsertRemovedSnapshot :exec
INSERT INTO snapshots (id, build_id, name, browser, viewport, baseline_id, status)
VALUES (@id, @build_id, @name, @browser, @viewport, @baseline_id, 'REMOVED')
ON CONFLICT (build_id, name, browser, viewport) DO NOTHING;
