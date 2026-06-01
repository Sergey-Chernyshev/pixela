// Package diffrun holds the River worker implementations for the diff pipeline (Phase 2): comparing a
// snapshot against its baseline, and recomputing a build's aggregate status once all its snapshots are
// terminal. It owns the workers' dependencies (db, object store, diff engine) and assembles the River
// workers bundle that internal/queue runs. See docs/spec/agents/04-diff-pipeline.md and rulebook §6/§10.
package diffrun

import (
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/diff"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/storage"
)

// Deps are the dependencies of the diff pipeline workers.
type Deps struct {
	DB     *db.DB
	Store  *storage.Store
	Engine diff.Engine
	Log    *slog.Logger

	// Options are the pinned pixelmatch knobs (per-project overrides land later).
	Options diff.Options
	// DiffRatioThreshold: a comparison whose changed-pixel ratio is <= this counts as UNCHANGED
	// (default 0 = any changed pixel is CHANGED). Independent of Options.PixelThreshold (spec §07).
	DiffRatioThreshold float64
}

// Workers builds the River workers bundle for the worker process. Defaults are filled for a nil engine.
func Workers(d Deps) *river.Workers {
	if d.Engine == nil {
		d.Engine = diff.NewStdlibEngine()
	}
	w := river.NewWorkers()
	river.AddWorker(w, &diffWorker{deps: d})
	river.AddWorker(w, &finalizeWorker{db: d.DB, log: d.Log})
	return w
}
