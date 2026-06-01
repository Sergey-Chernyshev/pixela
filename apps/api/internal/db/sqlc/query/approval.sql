-- Approval workflow (Phase 5): approve/reject snapshots, move the git-native baseline (Mode A), and
-- recompute the build's aggregate status. Approve of a CHANGED/NEW snapshot promotes its new image to
-- the (project, branch, name, browser, viewport) baseline; approve of a REMOVED snapshot accepts the
-- deletion and drops the baseline; reject marks the snapshot REJECTED. Membership is enforced by the
-- join (a non-member resolves to no row → 403/404 at the handler).

-- name: GetSnapshotForReview :one
SELECT
  s.id, s.build_id, b.project_id, b.branch,
  s.name, s.browser, s.viewport, s.new_image_sha, s.status
FROM snapshots s
JOIN builds b ON b.id = s.build_id
JOIN memberships m ON m.project_id = b.project_id
WHERE s.id = @snapshot_id AND m.user_id = @user_id;

-- All snapshots in a build that a reviewer can act on (batch approve/reject). Build membership is
-- checked separately via GetBuildForMember before this runs.
-- name: ListReviewableSnapshotsForBuild :many
SELECT s.id, s.name, s.browser, s.viewport, s.new_image_sha, s.status, b.project_id, b.branch
FROM snapshots s
JOIN builds b ON b.id = s.build_id
WHERE s.build_id = @build_id AND s.status IN ('CHANGED', 'NEW', 'REMOVED', 'ERROR')
ORDER BY s.name, s.browser, s.viewport;

-- name: SetSnapshotApproved :exec
UPDATE snapshots SET status = 'APPROVED' WHERE id = @id;

-- name: SetSnapshotRejected :exec
UPDATE snapshots SET status = 'REJECTED' WHERE id = @id;

-- Promote (or move) the baseline for a snapshot's identity key. The unique key makes this an upsert,
-- so re-approving the same key just advances image_sha + provenance.
-- name: UpsertBaseline :exec
INSERT INTO baselines (id, project_id, branch, name, browser, viewport, image_sha, approved_by_user_id, approved_in_build_id)
VALUES (@id, @project_id, @branch, @name, @browser, @viewport, @image_sha, @approved_by_user_id, @approved_in_build_id)
ON CONFLICT (project_id, branch, name, browser, viewport)
DO UPDATE SET
  image_sha = EXCLUDED.image_sha,
  approved_by_user_id = EXCLUDED.approved_by_user_id,
  approved_in_build_id = EXCLUDED.approved_in_build_id,
  updated_at = now();

-- Drop a baseline when an approved REMOVED snapshot accepts the deletion.
-- name: DeleteBaselineForKey :exec
DELETE FROM baselines
WHERE project_id = @project_id AND branch = @branch
  AND name = @name AND browser = @browser AND viewport = @viewport;

-- name: InsertApprovalEvent :exec
INSERT INTO approval_events (id, snapshot_id, user_id, action)
VALUES (@id, @snapshot_id, @user_id, @action);

-- Rejected snapshots in a build (a single rejection moves the whole build to REJECTED).
-- name: CountRejectedSnapshots :one
SELECT count(*) FROM snapshots WHERE build_id = @build_id AND status = 'REJECTED';
