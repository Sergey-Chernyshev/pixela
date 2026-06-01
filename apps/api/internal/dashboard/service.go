// Package dashboard implements the human-facing read/review surface: session login, the project/build/
// snapshot reads, and presigned image URLs. Every multi-project read is scoped to the caller's
// memberships at the QUERY level (spec §10 F-36) — a non-member can never observe another project's
// data. HTTP concerns (cookies, Huma ops) live in httpapi; this package returns plain view structs.
package dashboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/auth"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/session"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/storage"
)

const buildsPageSize = 20

// Service orchestrates the dashboard reads over the DB, the session store and the object store.
type Service struct {
	db         *db.DB
	sessions   *session.Store
	store      *storage.Store
	log        *slog.Logger
	presignTTL time.Duration
}

// NewService wires the dashboard service.
func NewService(database *db.DB, sessions *session.Store, store *storage.Store, presignTTL time.Duration, log *slog.Logger) *Service {
	return &Service{db: database, sessions: sessions, store: store, presignTTL: presignTTL, log: log}
}

// ---- auth ----

// User is the current dashboard user.
type User struct {
	ID    string  `json:"id"`
	Email string  `json:"email"`
	Name  *string `json:"name,omitempty"`
}

// Login verifies credentials and mints a session. The error is the generic ErrInvalidCredentials for
// both unknown email and wrong password, so the response never reveals whether an email exists.
func (s *Service) Login(ctx context.Context, email, password string) (auth.UserPrincipal, string, error) {
	user, err := s.db.Queries().GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.UserPrincipal{}, "", core.ErrInvalidCredentials
	}
	if err != nil {
		return auth.UserPrincipal{}, "", fmt.Errorf("get user: %w", err)
	}
	if user.PasswordHash == nil {
		return auth.UserPrincipal{}, "", core.ErrInvalidCredentials // OAuth-only account, no password
	}
	ok, verr := auth.VerifyPassword(*user.PasswordHash, password)
	if verr != nil {
		s.log.WarnContext(ctx, "password hash parse failed", "user_id", user.ID, "error", verr.Error())
	}
	if !ok {
		return auth.UserPrincipal{}, "", core.ErrInvalidCredentials
	}
	sid, err := s.sessions.Create(ctx, user.ID)
	if err != nil {
		return auth.UserPrincipal{}, "", fmt.Errorf("create session: %w", err)
	}
	return auth.UserPrincipal{UserID: user.ID, Email: user.Email}, sid, nil
}

// Logout destroys the session id (idempotent).
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.sessions.Destroy(ctx, sessionID)
}

// Me returns the current user's profile.
func (s *Service) Me(ctx context.Context, userID string) (User, error) {
	u, err := s.db.Queries().GetUserByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, fmt.Errorf("user: %w", core.ErrNotFound)
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return User{ID: u.ID, Email: u.Email, Name: u.Name}, nil
}

// ---- projects ----

// ProjectView is a project the user belongs to.
type ProjectView struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	DefaultBranch string    `json:"defaultBranch"`
	Role          string    `json:"role"`
	CreatedAt     time.Time `json:"createdAt"`
}

// ListProjects returns the projects the user is a member of.
func (s *Service) ListProjects(ctx context.Context, userID string) ([]ProjectView, error) {
	rows, err := s.db.Queries().ListProjectsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	out := make([]ProjectView, 0, len(rows))
	for _, r := range rows {
		out = append(out, ProjectView{
			ID: r.ID, Name: r.Name, Slug: r.Slug, DefaultBranch: r.DefaultBranch,
			Role: string(r.Role), CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// ---- builds ----

// Counts are the per-status snapshot counts shown on a build.
type Counts struct {
	Unchanged int `json:"unchanged"`
	Changed   int `json:"changed"`
	New       int `json:"new"`
	Removed   int `json:"removed"`
}

// BuildListItem is one row of a project's build feed.
type BuildListItem struct {
	ID        string    `json:"id"`
	Branch    string    `json:"branch"`
	CommitSha string    `json:"commitSha"`
	Status    string    `json:"status"`
	Counts    Counts    `json:"counts"`
	CreatedAt time.Time `json:"createdAt"`
	CIJobURL  *string   `json:"ciJobUrl,omitempty"`
}

// BuildsPage is a paginated build feed.
type BuildsPage struct {
	Items      []BuildListItem `json:"items"`
	Page       int             `json:"page"`
	TotalPages int             `json:"totalPages"`
}

// ListBuilds returns a project's build feed (membership-enforced). branch/status are optional filters,
// page is 1-based.
func (s *Service) ListBuilds(ctx context.Context, userID, projectID, branch, status string, page int) (BuildsPage, error) {
	member, err := s.db.Queries().IsProjectMember(ctx, db.IsProjectMemberParams{UserID: userID, ProjectID: projectID})
	if err != nil {
		return BuildsPage{}, fmt.Errorf("membership check: %w", err)
	}
	if !member {
		return BuildsPage{}, fmt.Errorf("project %s: %w", projectID, core.ErrForbiddenProject)
	}
	if page < 1 {
		page = 1
	}

	rows, err := s.db.Queries().ListBuildsForProjectMember(ctx, db.ListBuildsForProjectMemberParams{
		ProjectID: projectID, UserID: userID, Branch: branch, Status: status,
		PageLimit: buildsPageSize, PageOffset: int32((page - 1) * buildsPageSize), //nolint:gosec // bounded page index
	})
	if err != nil {
		return BuildsPage{}, fmt.Errorf("list builds: %w", err)
	}
	total, err := s.db.Queries().CountBuildsForProjectMember(ctx, db.CountBuildsForProjectMemberParams{
		ProjectID: projectID, UserID: userID, Branch: branch, Status: status,
	})
	if err != nil {
		return BuildsPage{}, fmt.Errorf("count builds: %w", err)
	}

	items := make([]BuildListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, BuildListItem{
			ID: r.ID, Branch: r.Branch, CommitSha: r.CommitSha, Status: string(r.Status),
			Counts:    Counts{Unchanged: int(r.Unchanged), Changed: int(r.Changed), New: int(r.New), Removed: int(r.Removed)},
			CreatedAt: r.CreatedAt.Time, CIJobURL: r.CiJobUrl,
		})
	}
	totalPages := int((total + buildsPageSize - 1) / buildsPageSize)
	if totalPages == 0 {
		totalPages = 1
	}
	return BuildsPage{Items: items, Page: page, TotalPages: totalPages}, nil
}

// SnapshotBrief is the snapshot metadata in a build detail.
type SnapshotBrief struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Browser   string   `json:"browser"`
	Viewport  string   `json:"viewport"`
	Status    string   `json:"status"`
	DiffRatio *float64 `json:"diffRatio,omitempty"`
}

// BuildDetail is a build with its snapshots.
type BuildDetail struct {
	ID        string          `json:"id"`
	Branch    string          `json:"branch"`
	CommitSha string          `json:"commitSha"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"createdAt"`
	CIJobURL  *string         `json:"ciJobUrl,omitempty"`
	Snapshots []SnapshotBrief `json:"snapshots"`
}

// GetBuild returns a build and its snapshots. A precise 403 (member of nothing) vs 404 (no such build).
func (s *Service) GetBuild(ctx context.Context, userID, buildID string) (BuildDetail, error) {
	b, err := s.db.Queries().GetBuildForMember(ctx, db.GetBuildForMemberParams{BuildID: buildID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return BuildDetail{}, s.notFoundOrForbidden(ctx, buildID, core.ErrBuildNotFound, s.db.Queries().GetBuildProject)
	}
	if err != nil {
		return BuildDetail{}, fmt.Errorf("get build: %w", err)
	}
	snaps, err := s.db.Queries().ListSnapshotsForBuild(ctx, buildID)
	if err != nil {
		return BuildDetail{}, fmt.Errorf("list snapshots: %w", err)
	}
	briefs := make([]SnapshotBrief, 0, len(snaps))
	for _, sn := range snaps {
		briefs = append(briefs, SnapshotBrief{
			ID: sn.ID, Name: sn.Name, Browser: sn.Browser, Viewport: sn.Viewport,
			Status: string(sn.Status), DiffRatio: sn.DiffRatio,
		})
	}
	return BuildDetail{
		ID: b.ID, Branch: b.Branch, CommitSha: b.CommitSha, Status: string(b.Status),
		CreatedAt: b.CreatedAt.Time, CIJobURL: b.CiJobUrl, Snapshots: briefs,
	}, nil
}

// ---- snapshot review ----

// Images are short-lived presigned URLs for the review viewer (null per status).
type Images struct {
	Baseline *string `json:"baseline"`
	New      *string `json:"new"`
	Diff     *string `json:"diff"`
}

// ApprovalEntry is one row of a snapshot's approval history.
type ApprovalEntry struct {
	Action string    `json:"action"`
	User   string    `json:"user"`
	At     time.Time `json:"at"`
}

// SnapshotReview is everything the review UI needs for one snapshot.
type SnapshotReview struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Browser    string          `json:"browser"`
	Viewport   string          `json:"viewport"`
	Status     string          `json:"status"`
	DiffRatio  *float64        `json:"diffRatio,omitempty"`
	DiffPixels *int32          `json:"diffPixels,omitempty"`
	Images     Images          `json:"images"`
	History    []ApprovalEntry `json:"history"`
}

// GetSnapshot returns the full review payload, with presigned image URLs. Precise 403 vs 404.
func (s *Service) GetSnapshot(ctx context.Context, userID, snapshotID string) (SnapshotReview, error) {
	r, err := s.db.Queries().GetSnapshotForMember(ctx, db.GetSnapshotForMemberParams{SnapshotID: snapshotID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return SnapshotReview{}, s.notFoundOrForbidden(ctx, snapshotID, core.ErrNotFound, s.db.Queries().GetSnapshotProject)
	}
	if err != nil {
		return SnapshotReview{}, fmt.Errorf("get snapshot: %w", err)
	}

	baseline, err := s.presign(ctx, r.BaselineImageSha)
	if err != nil {
		return SnapshotReview{}, err
	}
	newURL, err := s.presign(ctx, r.NewImageSha)
	if err != nil {
		return SnapshotReview{}, err
	}
	diffURL, err := s.presign(ctx, r.DiffImageSha)
	if err != nil {
		return SnapshotReview{}, err
	}

	events, err := s.db.Queries().ListApprovalEvents(ctx, snapshotID)
	if err != nil {
		return SnapshotReview{}, fmt.Errorf("list approval events: %w", err)
	}
	history := make([]ApprovalEntry, 0, len(events))
	for _, e := range events {
		history = append(history, ApprovalEntry{Action: string(e.Action), User: e.UserEmail, At: e.CreatedAt.Time})
	}

	return SnapshotReview{
		ID: r.ID, Name: r.Name, Browser: r.Browser, Viewport: r.Viewport, Status: string(r.Status),
		DiffRatio: r.DiffRatio, DiffPixels: r.DiffPixels,
		Images:  Images{Baseline: baseline, New: newURL, Diff: diffURL},
		History: history,
	}, nil
}

// presign returns a short-lived URL for a blob sha, or nil if the sha is nil.
func (s *Service) presign(ctx context.Context, sha *string) (*string, error) {
	if sha == nil {
		return nil, nil
	}
	url, err := s.store.PresignedGetURL(ctx, *sha, s.presignTTL)
	if err != nil {
		return nil, fmt.Errorf("presign %s: %w", *sha, err)
	}
	return &url, nil
}

// notFoundOrForbidden distinguishes "resource does not exist" (notFoundErr) from "exists but you are not
// a member" (ErrForbiddenProject) — so we never leak existence to non-members beyond a 403.
func (s *Service) notFoundOrForbidden(ctx context.Context, id string, notFoundErr error, projectOf func(context.Context, string) (string, error)) error {
	_, err := projectOf(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", id, notFoundErr)
	}
	if err != nil {
		return fmt.Errorf("resolve owner: %w", err)
	}
	return fmt.Errorf("%s: %w", id, core.ErrForbiddenProject)
}
