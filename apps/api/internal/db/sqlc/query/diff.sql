-- Diff pipeline (Phase 2): baseline resolution, per-snapshot result, build-status recompute.

-- name: GetSnapshot :one
SELECT * FROM snapshots WHERE id = @id;

-- Baseline resolution: strictly within the branch (NO merge-base; invariant #1).
-- name: GetBaselineForKey :one
SELECT * FROM baselines
WHERE project_id = @project_id AND branch = @branch
  AND name = @name AND browser = @browser AND viewport = @viewport;

-- name: SetSnapshotNew :exec
UPDATE snapshots
SET status = 'NEW', diff_ratio = NULL, diff_pixels = NULL, diff_image_sha = NULL
WHERE id = @id;

-- name: SetSnapshotUnchanged :exec
UPDATE snapshots
SET status = 'UNCHANGED', diff_ratio = 0, diff_pixels = 0, diff_image_sha = NULL, baseline_id = @baseline_id
WHERE id = @id;

-- name: SetSnapshotChanged :exec
UPDATE snapshots
SET status = 'CHANGED', diff_ratio = @diff_ratio, diff_pixels = @diff_pixels,
    diff_image_sha = @diff_image_sha, baseline_id = @baseline_id
WHERE id = @id;

-- name: SetSnapshotError :exec
UPDATE snapshots SET status = 'ERROR', error_msg = @error_msg WHERE id = @id;

-- name: CountPendingSnapshots :one
SELECT count(*) FROM snapshots WHERE build_id = @build_id AND status = 'PENDING';

-- Reviewable = anything that blocks a clean pass (CHANGED/NEW/REMOVED/ERROR). Approved changes count
-- as resolved, so they do NOT block PASSED.
-- name: CountReviewableSnapshots :one
SELECT count(*) FROM snapshots
WHERE build_id = @build_id AND status IN ('CHANGED', 'NEW', 'REMOVED', 'ERROR');

-- name: SetBuildStatus :exec
UPDATE builds SET status = @status WHERE id = @id;
