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
	"github.com/jackc/pgx/v5/pgtype"

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
	// decoyHash is a valid argon2id hash computed once at startup. Login verifies against it on the
	// unknown-email / null-password paths so every failure pays the same KDF cost — closing the timing
	// side-channel that would otherwise let an attacker enumerate accounts despite the generic error.
	decoyHash string
}

// NewService wires the dashboard service. It precomputes the login decoy hash; a crypto/rand failure
// here aborts startup (the alternative — skipping it — would silently reopen the enumeration oracle).
func NewService(database *db.DB, sessions *session.Store, store *storage.Store, presignTTL time.Duration, log *slog.Logger) (*Service, error) {
	decoy, err := auth.HashPassword(core.NewID())
	if err != nil {
		return nil, fmt.Errorf("init login decoy hash: %w", err)
	}
	return &Service{
		db: database, sessions: sessions, store: store, presignTTL: presignTTL, log: log, decoyHash: decoy,
	}, nil
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
		s.equalizeLoginTiming(password) // unknown email: still pay the KDF cost (anti-enumeration)
		return auth.UserPrincipal{}, "", core.ErrInvalidCredentials
	}
	if err != nil {
		return auth.UserPrincipal{}, "", fmt.Errorf("get user: %w", err)
	}
	if user.PasswordHash == nil {
		s.equalizeLoginTiming(password) // OAuth-only account, no password: pay the same cost
		return auth.UserPrincipal{}, "", core.ErrInvalidCredentials
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

// equalizeLoginTiming runs the argon2 KDF against the decoy hash and discards the result, so a failed
// login on the no-such-user / no-password path takes the same time as a wrong-password verification.
// Without this, response latency leaks whether an email is a registered, password-backed account.
func (s *Service) equalizeLoginTiming(password string) {
	_, _ = auth.VerifyPassword(s.decoyHash, password)
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

// ProjectView is a project the user belongs to, enriched with real aggregates computed in SQL: open
// reviews, member count, last-build time/status, and the latest build's in-norm snapshot ratio (for the
// health bar). Every figure comes from existing rows — nothing is synthesised.
type ProjectView struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Slug            string     `json:"slug"`
	DefaultBranch   string     `json:"defaultBranch"`
	Role            string     `json:"role"`
	CreatedAt       time.Time  `json:"createdAt"`
	OpenReviews     int        `json:"openReviews"`
	MemberCount     int        `json:"memberCount"`
	LastBuildAt     *time.Time `json:"lastBuildAt,omitempty"`
	LastBuildStatus *string    `json:"lastBuildStatus,omitempty"`
	HealthOk        int        `json:"healthOk"`
	HealthTotal     int        `json:"healthTotal"`
}

// ListProjects returns the projects the user is a member of, with overview aggregates.
func (s *Service) ListProjects(ctx context.Context, userID string) ([]ProjectView, error) {
	rows, err := s.db.Queries().ListProjectsOverview(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	out := make([]ProjectView, 0, len(rows))
	for _, r := range rows {
		pv := ProjectView{
			ID: r.ID, Name: r.Name, Slug: r.Slug, DefaultBranch: r.DefaultBranch,
			Role: string(r.Role), CreatedAt: r.CreatedAt.Time,
			OpenReviews: int(r.OpenReviews), MemberCount: int(r.MemberCount),
			LastBuildAt: tsPtr(r.LastBuildAt),
			HealthOk:    int(r.HealthOk), HealthTotal: int(r.HealthTotal),
		}
		if r.LastBuildAt.Valid {
			status := string(r.LastBuildStatus)
			pv.LastBuildStatus = &status
		}
		out = append(out, pv)
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

// BuildListItem is one row of a project's build feed. FinalizedAt (when set) lets the UI show the run
// duration (finalizedAt − createdAt) — a real value, not a fabricated one.
type BuildListItem struct {
	ID          string     `json:"id"`
	Branch      string     `json:"branch"`
	CommitSha   string     `json:"commitSha"`
	Status      string     `json:"status"`
	Counts      Counts     `json:"counts"`
	CreatedAt   time.Time  `json:"createdAt"`
	FinalizedAt *time.Time `json:"finalizedAt,omitempty"`
	CIJobURL    *string    `json:"ciJobUrl,omitempty"`
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

	// Count first so we can clamp the requested page to [1, totalPages] BEFORE computing the offset.
	// This keeps the response consistent (Page <= TotalPages) and guarantees the offset is bounded by
	// the row count — never a negative/overflowed OFFSET that Postgres would reject (a 500).
	total, err := s.db.Queries().CountBuildsForProjectMember(ctx, db.CountBuildsForProjectMemberParams{
		ProjectID: projectID, UserID: userID, Branch: branch, Status: status,
	})
	if err != nil {
		return BuildsPage{}, fmt.Errorf("count builds: %w", err)
	}
	totalPages := int((total + buildsPageSize - 1) / buildsPageSize)
	if totalPages < 1 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	rows, err := s.db.Queries().ListBuildsForProjectMember(ctx, db.ListBuildsForProjectMemberParams{
		ProjectID: projectID, UserID: userID, Branch: branch, Status: status,
		//nolint:gosec // page is clamped to [1, totalPages] above, so the offset is bounded by `total`
		PageLimit: buildsPageSize, PageOffset: int32((page - 1) * buildsPageSize),
	})
	if err != nil {
		return BuildsPage{}, fmt.Errorf("list builds: %w", err)
	}

	items := make([]BuildListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, BuildListItem{
			ID: r.ID, Branch: r.Branch, CommitSha: r.CommitSha, Status: string(r.Status),
			Counts:    Counts{Unchanged: int(r.Unchanged), Changed: int(r.Changed), New: int(r.New), Removed: int(r.Removed)},
			CreatedAt: r.CreatedAt.Time, FinalizedAt: tsPtr(r.FinalizedAt), CIJobURL: r.CiJobUrl,
		})
	}
	return BuildsPage{Items: items, Page: page, TotalPages: totalPages}, nil
}

// SnapshotBrief is the snapshot metadata in a build detail, with presigned thumbnail URLs (baseline /
// new / diff, any may be null per status) so the grid can render a real preview per card.
type SnapshotBrief struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Browser   string   `json:"browser"`
	Viewport  string   `json:"viewport"`
	Status    string   `json:"status"`
	DiffRatio *float64 `json:"diffRatio,omitempty"`
	Images    Images   `json:"images"`
}

// BuildDetail is a build with its snapshots. FinalizedAt (when set) gives the real run duration.
type BuildDetail struct {
	ID          string          `json:"id"`
	Branch      string          `json:"branch"`
	CommitSha   string          `json:"commitSha"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"createdAt"`
	FinalizedAt *time.Time      `json:"finalizedAt,omitempty"`
	CIJobURL    *string         `json:"ciJobUrl,omitempty"`
	Snapshots   []SnapshotBrief `json:"snapshots"`
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
		imgs, err := s.presignTrio(ctx, sn.BaselineImageSha, sn.NewImageSha, sn.DiffImageSha)
		if err != nil {
			return BuildDetail{}, err
		}
		briefs = append(briefs, SnapshotBrief{
			ID: sn.ID, Name: sn.Name, Browser: sn.Browser, Viewport: sn.Viewport,
			Status: string(sn.Status), DiffRatio: sn.DiffRatio, Images: imgs,
		})
	}
	return BuildDetail{
		ID: b.ID, Branch: b.Branch, CommitSha: b.CommitSha, Status: string(b.Status),
		CreatedAt: b.CreatedAt.Time, FinalizedAt: tsPtr(b.FinalizedAt), CIJobURL: b.CiJobUrl, Snapshots: briefs,
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

// presignTrio presigns the baseline/new/diff blobs of one snapshot for the build-detail thumbnail grid.
func (s *Service) presignTrio(ctx context.Context, baseline, newSha, diff *string) (Images, error) {
	b, err := s.presign(ctx, baseline)
	if err != nil {
		return Images{}, err
	}
	n, err := s.presign(ctx, newSha)
	if err != nil {
		return Images{}, err
	}
	d, err := s.presign(ctx, diff)
	if err != nil {
		return Images{}, err
	}
	return Images{Baseline: b, New: n, Diff: d}, nil
}

// requireMember gates a project-scoped list endpoint: a non-member gets ErrForbiddenProject (we do not
// distinguish a missing project from an inaccessible one here — neither is observable beyond a 403).
func (s *Service) requireMember(ctx context.Context, userID, projectID string) error {
	member, err := s.db.Queries().IsProjectMember(ctx, db.IsProjectMemberParams{UserID: userID, ProjectID: projectID})
	if err != nil {
		return fmt.Errorf("membership check: %w", err)
	}
	if !member {
		return fmt.Errorf("project %s: %w", projectID, core.ErrForbiddenProject)
	}
	return nil
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

// tsPtr converts a nullable Postgres timestamp into a *time.Time (nil when the column is NULL).
func tsPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

// ---- members ----

// Member is one row of a project's members table (Участники).
type Member struct {
	ID           string  `json:"id"`
	Email        string  `json:"email"`
	Name         *string `json:"name,omitempty"`
	Role         string  `json:"role"`
	TotalReviews int     `json:"totalReviews"`
}

// ListMembers returns a project's members (membership-enforced) with each member's all-time review tally.
func (s *Service) ListMembers(ctx context.Context, userID, projectID string) ([]Member, error) {
	if err := s.requireMember(ctx, userID, projectID); err != nil {
		return nil, err
	}
	rows, err := s.db.Queries().ListProjectMembers(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	out := make([]Member, 0, len(rows))
	for _, r := range rows {
		out = append(out, Member{
			ID: r.ID, Email: r.Email, Name: r.Name, Role: string(r.Role), TotalReviews: int(r.TotalReviews),
		})
	}
	return out, nil
}

// ---- baselines ----

// BaselineView is one accepted per-branch baseline (Базовые линии) with a presigned thumbnail.
type BaselineView struct {
	ID         string    `json:"id"`
	Branch     string    `json:"branch"`
	Name       string    `json:"name"`
	Browser    string    `json:"browser"`
	Viewport   string    `json:"viewport"`
	ImageURL   *string   `json:"imageUrl"`
	ApprovedBy *string   `json:"approvedBy,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt"`
	CreatedAt  time.Time `json:"createdAt"`
}

// ListBaselines returns a project's per-branch baselines (membership-enforced), each with a presigned
// thumbnail URL for its current blob.
func (s *Service) ListBaselines(ctx context.Context, userID, projectID string) ([]BaselineView, error) {
	if err := s.requireMember(ctx, userID, projectID); err != nil {
		return nil, err
	}
	rows, err := s.db.Queries().ListProjectBaselines(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list baselines: %w", err)
	}
	out := make([]BaselineView, 0, len(rows))
	for _, r := range rows {
		sha := r.ImageSha
		url, err := s.presign(ctx, &sha)
		if err != nil {
			return nil, err
		}
		out = append(out, BaselineView{
			ID: r.ID, Branch: r.Branch, Name: r.Name, Browser: r.Browser, Viewport: r.Viewport,
			ImageURL: url, ApprovedBy: r.ApprovedByEmail, UpdatedAt: r.UpdatedAt.Time, CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

// ---- activity ----

const activityLimit = 60

// ActivityEntry is one organization-wide approval event (Активность).
type ActivityEntry struct {
	ID           string    `json:"id"`
	Action       string    `json:"action"`
	User         string    `json:"user"`
	SnapshotID   string    `json:"snapshotId"`
	SnapshotName string    `json:"snapshotName"`
	Branch       string    `json:"branch"`
	ProjectID    string    `json:"projectId"`
	ProjectName  string    `json:"projectName"`
	At           time.Time `json:"at"`
}

// ListActivity returns the recent approval-event feed across every project the user belongs to.
func (s *Service) ListActivity(ctx context.Context, userID string) ([]ActivityEntry, error) {
	rows, err := s.db.Queries().ListActivityForUser(ctx, db.ListActivityForUserParams{
		UserID: userID, PageLimit: activityLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list activity: %w", err)
	}
	out := make([]ActivityEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, ActivityEntry{
			ID: r.ID, Action: string(r.Action), User: r.UserEmail,
			SnapshotID: r.SnapshotID, SnapshotName: r.SnapshotName, Branch: r.Branch,
			ProjectID: r.ProjectID, ProjectName: r.ProjectName, At: r.CreatedAt.Time,
		})
	}
	return out, nil
}
