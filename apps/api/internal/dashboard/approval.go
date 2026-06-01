package dashboard

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
)

// ReviewResult is returned to the UI after an approve/reject so it can reflect the build's new status
// (and how many snapshots were affected) without a refetch.
type ReviewResult struct {
	BuildID     string `json:"buildId"`
	BuildStatus string `json:"buildStatus"`
	Affected    int    `json:"affected"`
}

// reviewSnap is the minimal snapshot identity an approve/reject needs, adapted from either the single
// (GetSnapshotForReview) or the batch (ListReviewableSnapshotsForBuild) query rows.
type reviewSnap struct {
	id, projectID, branch, name, browser, viewport string
	newImageSha                                    *string
	status                                         string
}

// ApproveSnapshot promotes one snapshot's new image to the (project, branch, name, browser, viewport)
// baseline (Mode A; REMOVED accepts the deletion and drops the baseline) and recomputes the build.
func (s *Service) ApproveSnapshot(ctx context.Context, userID, snapshotID string) (ReviewResult, error) {
	return s.reviewOne(ctx, userID, snapshotID, db.ApprovalActionAPPROVE)
}

// RejectSnapshot marks one snapshot REJECTED (the baseline is untouched) and recomputes the build.
func (s *Service) RejectSnapshot(ctx context.Context, userID, snapshotID string) (ReviewResult, error) {
	return s.reviewOne(ctx, userID, snapshotID, db.ApprovalActionREJECT)
}

func (s *Service) reviewOne(ctx context.Context, userID, snapshotID string, action db.ApprovalAction) (ReviewResult, error) {
	row, err := s.db.Queries().GetSnapshotForReview(ctx, db.GetSnapshotForReviewParams{SnapshotID: snapshotID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewResult{}, s.notFoundOrForbidden(ctx, snapshotID, core.ErrNotFound, s.db.Queries().GetSnapshotProject)
	}
	if err != nil {
		return ReviewResult{}, fmt.Errorf("get snapshot for review: %w", err)
	}
	rs := reviewSnap{
		id: row.ID, projectID: row.ProjectID, branch: row.Branch, name: row.Name,
		browser: row.Browser, viewport: row.Viewport, newImageSha: row.NewImageSha, status: string(row.Status),
	}
	return s.applyReview(ctx, userID, row.BuildID, []reviewSnap{rs}, action)
}

// ApproveBuild / RejectBuild act on every reviewable snapshot of a build in one transaction.
func (s *Service) ApproveBuild(ctx context.Context, userID, buildID string) (ReviewResult, error) {
	return s.reviewBuild(ctx, userID, buildID, db.ApprovalActionAPPROVE)
}

func (s *Service) RejectBuild(ctx context.Context, userID, buildID string) (ReviewResult, error) {
	return s.reviewBuild(ctx, userID, buildID, db.ApprovalActionREJECT)
}

func (s *Service) reviewBuild(ctx context.Context, userID, buildID string, action db.ApprovalAction) (ReviewResult, error) {
	if _, err := s.db.Queries().GetBuildForMember(ctx, db.GetBuildForMemberParams{BuildID: buildID, UserID: userID}); errors.Is(err, pgx.ErrNoRows) {
		return ReviewResult{}, s.notFoundOrForbidden(ctx, buildID, core.ErrBuildNotFound, s.db.Queries().GetBuildProject)
	} else if err != nil {
		return ReviewResult{}, fmt.Errorf("get build for member: %w", err)
	}
	rows, err := s.db.Queries().ListReviewableSnapshotsForBuild(ctx, buildID)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("list reviewable: %w", err)
	}
	snaps := make([]reviewSnap, 0, len(rows))
	for _, r := range rows {
		// ERROR snapshots can be rejected but never approved (there is no good image to baseline) — skip
		// them in a batch APPROVE so one broken snapshot doesn't block accepting the rest.
		if action == db.ApprovalActionAPPROVE && r.Status == db.SnapshotStatusERROR {
			continue
		}
		snaps = append(snaps, reviewSnap{
			id: r.ID, projectID: r.ProjectID, branch: r.Branch, name: r.Name,
			browser: r.Browser, viewport: r.Viewport, newImageSha: r.NewImageSha, status: string(r.Status),
		})
	}
	return s.applyReview(ctx, userID, buildID, snaps, action)
}

// applyReview runs the per-snapshot mutations and recomputes the build's aggregate status in ONE
// transaction (the build row is locked FOR UPDATE so concurrent reviews/finalizes serialize).
func (s *Service) applyReview(ctx context.Context, userID, buildID string, snaps []reviewSnap, action db.ApprovalAction) (ReviewResult, error) {
	tx, err := s.db.Pool().Begin(ctx)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("begin review tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.db.Queries().WithTx(tx)

	if _, err := qtx.GetBuildForUpdate(ctx, buildID); err != nil {
		return ReviewResult{}, fmt.Errorf("lock build: %w", err)
	}

	affected := 0
	for _, rs := range snaps {
		if err := s.applyOne(ctx, qtx, userID, buildID, rs, action); err != nil {
			return ReviewResult{}, err
		}
		affected++
	}

	status, err := recomputeBuildStatus(ctx, qtx, buildID)
	if err != nil {
		return ReviewResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReviewResult{}, fmt.Errorf("commit review: %w", err)
	}
	return ReviewResult{BuildID: buildID, BuildStatus: string(status), Affected: affected}, nil
}

func (s *Service) applyOne(ctx context.Context, qtx *db.Queries, userID, buildID string, rs reviewSnap, action db.ApprovalAction) error {
	if !isReviewable(rs.status) {
		return fmt.Errorf("snapshot %s is %s, not reviewable: %w", rs.id, rs.status, core.ErrConflict)
	}

	if action == db.ApprovalActionAPPROVE {
		switch rs.status {
		case string(db.SnapshotStatusCHANGED), string(db.SnapshotStatusNEW):
			if rs.newImageSha == nil {
				return fmt.Errorf("snapshot %s has no image to approve: %w", rs.id, core.ErrConflict)
			}
			if err := qtx.UpsertBaseline(ctx, db.UpsertBaselineParams{
				ID: core.NewID(), ProjectID: rs.projectID, Branch: rs.branch, Name: rs.name,
				Browser: rs.browser, Viewport: rs.viewport, ImageSha: *rs.newImageSha,
				ApprovedByUserID: &userID, ApprovedInBuildID: &buildID,
			}); err != nil {
				return fmt.Errorf("upsert baseline: %w", err)
			}
		case string(db.SnapshotStatusREMOVED):
			if err := qtx.DeleteBaselineForKey(ctx, db.DeleteBaselineForKeyParams{
				ProjectID: rs.projectID, Branch: rs.branch, Name: rs.name, Browser: rs.browser, Viewport: rs.viewport,
			}); err != nil {
				return fmt.Errorf("delete baseline: %w", err)
			}
		case string(db.SnapshotStatusERROR):
			return fmt.Errorf("snapshot %s is in ERROR and cannot be approved: %w", rs.id, core.ErrConflict)
		}
		if err := qtx.SetSnapshotApproved(ctx, rs.id); err != nil {
			return fmt.Errorf("set snapshot approved: %w", err)
		}
	} else {
		if err := qtx.SetSnapshotRejected(ctx, rs.id); err != nil {
			return fmt.Errorf("set snapshot rejected: %w", err)
		}
	}

	if err := qtx.InsertApprovalEvent(ctx, db.InsertApprovalEventParams{
		ID: core.NewID(), SnapshotID: rs.id, UserID: userID, Action: action,
	}); err != nil {
		return fmt.Errorf("insert approval event: %w", err)
	}
	return nil
}

// recomputeBuildStatus mirrors the diff finalize rule plus rejection: any REJECTED → REJECTED; else any
// still-reviewable (CHANGED/NEW/REMOVED/ERROR) → REVIEW_REQUIRED; else PASSED.
func recomputeBuildStatus(ctx context.Context, qtx *db.Queries, buildID string) (db.BuildStatus, error) {
	rejected, err := qtx.CountRejectedSnapshots(ctx, buildID)
	if err != nil {
		return "", fmt.Errorf("count rejected: %w", err)
	}
	var status db.BuildStatus
	switch {
	case rejected > 0:
		status = db.BuildStatusREJECTED
	default:
		reviewable, rerr := qtx.CountReviewableSnapshots(ctx, buildID)
		if rerr != nil {
			return "", fmt.Errorf("count reviewable: %w", rerr)
		}
		if reviewable > 0 {
			status = db.BuildStatusREVIEWREQUIRED
		} else {
			status = db.BuildStatusPASSED
		}
	}
	if err := qtx.SetBuildStatus(ctx, db.SetBuildStatusParams{Status: status, ID: buildID}); err != nil {
		return "", fmt.Errorf("set build status: %w", err)
	}
	return status, nil
}

func isReviewable(status string) bool {
	switch status {
	case string(db.SnapshotStatusCHANGED), string(db.SnapshotStatusNEW),
		string(db.SnapshotStatusREMOVED), string(db.SnapshotStatusERROR):
		return true
	default:
		return false
	}
}
