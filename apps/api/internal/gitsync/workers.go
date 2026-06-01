// Package gitsync holds the River workers that realize Mode A's git-native side effects (Phase 5b/5c):
// committing approved baselines back to a project's GitLab repo, and mirroring a build's review state to
// its commit/MR. Both are best-effort and no-op cleanly when the project has no GitLab repo wired or no
// token is configured — the core review flow never depends on GitLab being reachable (invariant #6).
package gitsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/gitlab"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/queue"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/storage"
)

// Deps are the dependencies of the git-sync workers.
type Deps struct {
	DB     *db.DB
	Store  *storage.Store
	GitLab *gitlab.Client
	Log    *slog.Logger
	// PublicURL is the dashboard's externally-reachable base (e.g. https://pixela.acme.dev); used to
	// build the commit-status target link. Optional.
	PublicURL string
}

// AddWorkers registers the git-sync workers into an existing bundle (merged with diffrun in the worker
// process).
func AddWorkers(w *river.Workers, d Deps) {
	river.AddWorker(w, &commitWorker{deps: d})
	river.AddWorker(w, &statusWorker{deps: d})
}

// ---- baseline commit ----

type commitWorker struct {
	river.WorkerDefaults[queue.GitCommitJobArgs]
	deps Deps
}

func (cw *commitWorker) Work(ctx context.Context, job *river.Job[queue.GitCommitJobArgs]) error {
	d := cw.deps
	if !d.GitLab.Enabled() {
		d.Log.InfoContext(ctx, "baseline commit skipped: GitLab token not configured", "build_id", job.Args.BuildID)
		return nil
	}
	q := d.DB.Queries()

	build, err := q.GetBuild(ctx, job.Args.BuildID)
	if err != nil {
		return fmt.Errorf("get build: %w", err)
	}
	proj, err := q.GetProjectGitlab(ctx, build.ProjectID)
	if err != nil {
		return fmt.Errorf("get project gitlab: %w", err)
	}
	if proj.GitlabProjectID == nil || *proj.GitlabProjectID == "" {
		d.Log.InfoContext(ctx, "baseline commit skipped: project has no GitLab repo", "project", proj.Slug)
		return nil
	}
	repo := *proj.GitlabProjectID

	authorName, authorEmail := "Pixela", "pixela@localhost"
	if u, uerr := q.GetUserByID(ctx, job.Args.UserID); uerr == nil {
		authorEmail = u.Email
		if u.Name != nil && *u.Name != "" {
			authorName = *u.Name
		}
	}

	for _, f := range job.Args.Files {
		var content []byte
		if f.Action != string(gitlab.ActionDelete) {
			content, err = d.Store.GetBytes(ctx, f.ImageSha)
			if err != nil {
				return fmt.Errorf("get baseline blob %s: %w", f.ImageSha, err)
			}
		}
		msg := fmt.Sprintf("chore(visual): approve baseline %s", f.Path)
		if err := d.GitLab.CommitFile(ctx, repo, build.Branch, gitlab.Action(f.Action), f.Path, content, msg, authorName, authorEmail); err != nil {
			return fmt.Errorf("commit baseline %s: %w", f.Path, err)
		}
		d.Log.InfoContext(ctx, "baseline committed", "repo", repo, "branch", build.Branch, "path", f.Path, "action", f.Action)
	}
	return nil
}

// ---- commit status mirror ----

type statusWorker struct {
	river.WorkerDefaults[queue.GitStatusJobArgs]
	deps Deps
}

func (sw *statusWorker) Work(ctx context.Context, job *river.Job[queue.GitStatusJobArgs]) error {
	d := sw.deps
	if !d.GitLab.Enabled() {
		return nil
	}
	q := d.DB.Queries()

	build, err := q.GetBuild(ctx, job.Args.BuildID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get build: %w", err)
	}
	proj, err := q.GetProjectGitlab(ctx, build.ProjectID)
	if err != nil {
		return fmt.Errorf("get project gitlab: %w", err)
	}
	if proj.GitlabProjectID == nil || *proj.GitlabProjectID == "" {
		return nil
	}

	state, desc := mapBuildState(string(build.Status))
	target := ""
	if d.PublicURL != "" {
		target = fmt.Sprintf("%s/builds/%s", d.PublicURL, build.ID)
	}
	if err := d.GitLab.SetCommitStatus(ctx, *proj.GitlabProjectID, build.CommitSha, state, "pixela/visual", target, desc); err != nil {
		return fmt.Errorf("set commit status: %w", err)
	}
	d.Log.InfoContext(ctx, "commit status mirrored", "commit", build.CommitSha, "state", state)
	return nil
}

// mapBuildState maps a Pixela build status to a GitLab commit state + human description.
func mapBuildState(status string) (gitlab.CommitState, string) {
	switch status {
	case string(db.BuildStatusPASSED):
		return gitlab.StateSuccess, "Visual review passed"
	case string(db.BuildStatusREJECTED), string(db.BuildStatusERROR):
		return gitlab.StateFailed, "Visual review: changes rejected or errored"
	default: // RUNNING / COMPARING / REVIEW_REQUIRED
		return gitlab.StatePending, "Visual review pending"
	}
}
