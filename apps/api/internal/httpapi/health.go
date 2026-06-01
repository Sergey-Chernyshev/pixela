package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
)

const readinessTimeout = 3 * time.Second

type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// handleLiveness is process-only: it never touches a dependency, so a transient DB/Redis blip cannot
// trigger a restart loop. Used by the container liveness probe.
func handleLiveness() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	}
}

// handleReadiness reports 200 only when the process is past startup AND every dependency check passes;
// otherwise 503 with the per-dependency status. Checks run concurrently under one bounded context.
func handleReadiness(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Ready != nil && !deps.Ready.Load() {
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "starting"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		checks := make(map[string]string, len(deps.Checkers))
		var mu sync.Mutex
		var wg sync.WaitGroup
		allUp := true

		for _, c := range deps.Checkers {
			wg.Add(1)
			go func(c core.HealthChecker) {
				defer wg.Done()
				status := "up"
				if err := c.Check(ctx); err != nil {
					status = "down"
					deps.Logger.WarnContext(ctx, "readiness check failed", "dep", c.Name(), "error", err.Error())
				}
				mu.Lock()
				checks[c.Name()] = status
				if status == "down" {
					allUp = false
				}
				mu.Unlock()
			}(c)
		}
		wg.Wait()

		code, status := http.StatusOK, "ok"
		if !allUp {
			code, status = http.StatusServiceUnavailable, "degraded"
		}
		writeJSON(w, code, healthResponse{Status: status, Checks: checks})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
