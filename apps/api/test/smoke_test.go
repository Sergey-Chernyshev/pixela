//go:build integration

// Package test holds the Phase-0 "green baseline" integration smoke: boot the binary's modes against
// ephemeral Postgres + Redis + MinIO, prove `pixela migrate` applies the schema on a clean DB and
// `pixela serve` reports /readyz 200 with every dependency up. Run: go test -tags=integration ./test/...
// (needs a Docker daemon). goleak is intentionally NOT used here — testcontainers/docker spawn their
// own goroutines; goleak belongs on the worker's own units (Phase 2).
package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/minio"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/app"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
)

func TestPhase0Smoke(t *testing.T) {
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	ctx := context.Background()

	// Start dependencies SEQUENTIALLY — concurrent first-time starts race on Docker Desktop's port
	// publishing ("No host port found for host IP").
	pg, err := postgres.Run(ctx, "postgres:16",
		postgres.WithDatabase("pixela"),
		postgres.WithUsername("pixela"),
		postgres.WithPassword("pixela"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = tc.TerminateContainer(pg) })

	rd, err := redis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatalf("start redis: %v", err)
	}
	t.Cleanup(func() { _ = tc.TerminateContainer(rd) })

	mn, err := minio.Run(ctx, "minio/minio:latest")
	if err != nil {
		t.Fatalf("start minio: %v", err)
	}
	t.Cleanup(func() { _ = tc.TerminateContainer(mn) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("pg dsn: %v", err)
	}
	redisURL, err := rd.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("redis url: %v", err)
	}
	minioEndpoint, err := mn.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("minio endpoint: %v", err)
	}

	port := freePort(t)
	env := map[string]string{
		"DATABASE_URL":   dsn,
		"REDIS_URL":      redisURL,
		"S3_ENDPOINT":    minioEndpoint,
		"S3_ACCESS_KEY":  mn.Username,
		"S3_SECRET_KEY":  mn.Password,
		"S3_BUCKET":      "pixela",
		"S3_USE_SSL":     "false",
		"SESSION_SECRET": "test-session-secret-not-a-real-one",
		"PORT":           fmt.Sprintf("%d", port),
		"PIXELA_ENV":     "test",
		"LOG_LEVEL":      "warn",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}

	// 1. migrate applies the schema (clean DB) + River tables. Idempotent: run twice.
	if err := app.Run(ctx, []string{"migrate"}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := app.Run(ctx, []string{"migrate"}); err != nil {
		t.Fatalf("migrate (idempotent re-run): %v", err)
	}

	// 2. schema is present and queryable via sqlc — Project table empty on a clean DB.
	database, err := db.Open(ctx, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	n, err := database.Queries().CountProjects(ctx)
	if err != nil {
		database.Close()
		t.Fatalf("count projects: %v", err)
	}
	database.Close()
	if n != 0 {
		t.Fatalf("expected 0 projects on clean DB, got %d", n)
	}

	// 3. serve, then /readyz must be 200 with every dependency up.
	serveCtx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() { errCh <- app.Run(serveCtx, []string{"serve"}) }()

	body := waitReady(t, port, 20*time.Second)
	if body.Status != "ok" {
		t.Fatalf("readyz status = %q, want ok (checks=%v)", body.Status, body.Checks)
	}
	for _, dep := range []string{"database", "redis", "objectstore"} {
		if got := body.Checks[dep]; got != "up" {
			t.Fatalf("readyz check %q = %q, want up", dep, got)
		}
	}

	// 4. graceful shutdown returns cleanly.
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serve returned error on shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not shut down within 10s")
	}
}

type readyz struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

func waitReady(t *testing.T, port int, within time.Duration) readyz {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/readyz", port)
	deadline := time.Now().Add(within)
	var last int
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:noctx // simple test poll
		if err == nil {
			last = resp.StatusCode
			var b readyz
			_ = json.NewDecoder(resp.Body).Decode(&b)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return b
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("/readyz never returned 200 within %s (last status %d)", within, last)
	return readyz{}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}
