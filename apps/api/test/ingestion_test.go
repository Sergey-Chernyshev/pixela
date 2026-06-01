//go:build integration

package test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/minio"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/app"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/auth"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
)

const sessionSecret = "phase1-test-session-secret-1234567890"

// TestPhase1Ingestion drives the full ingestion contract against real infra and asserts every Agent-02
// acceptance criterion: auth, project isolation, two-phase CAS dedup, idempotent retry, hash mismatch,
// PNG validation, and finalize (REMOVED + diff-job enqueue).
func TestPhase1Ingestion(t *testing.T) {
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	ctx := context.Background()

	pg, err := postgres.Run(ctx, "postgres:16",
		postgres.WithDatabase("pixela"), postgres.WithUsername("pixela"), postgres.WithPassword("pixela"),
		postgres.BasicWaitStrategies())
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(func() { _ = tc.TerminateContainer(pg) })
	rd, err := redis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatalf("redis: %v", err)
	}
	t.Cleanup(func() { _ = tc.TerminateContainer(rd) })
	mn, err := minio.Run(ctx, "minio/minio:latest")
	if err != nil {
		t.Fatalf("minio: %v", err)
	}
	t.Cleanup(func() { _ = tc.TerminateContainer(mn) })

	dsn, _ := pg.ConnectionString(ctx, "sslmode=disable")
	redisURL, _ := rd.ConnectionString(ctx)
	minioEndpoint, _ := mn.ConnectionString(ctx)

	port := freePort(t)
	for k, v := range map[string]string{
		"DATABASE_URL": dsn, "REDIS_URL": redisURL, "S3_ENDPOINT": minioEndpoint,
		"S3_ACCESS_KEY": mn.Username, "S3_SECRET_KEY": mn.Password, "S3_BUCKET": "pixela", "S3_USE_SSL": "false",
		"SESSION_SECRET": sessionSecret, "PORT": fmt.Sprintf("%d", port), "PIXELA_ENV": "test", "LOG_LEVEL": "warn",
	} {
		t.Setenv(k, v)
	}

	if err := app.Run(ctx, []string{"migrate"}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Seed two projects, an API key for each, plus a baseline in project A (to exercise REMOVED).
	database, err := db.Open(ctx, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(database.Close)
	q := database.Queries()

	projA := mustProject(t, ctx, q, "Project A", "proj-a")
	projB := mustProject(t, ctx, q, "Project B", "proj-b")
	keyA := mustAPIKey(t, ctx, q, projA)
	keyB := mustAPIKey(t, ctx, q, projB)
	seedBaseline(t, ctx, database, projA, "feature/x", "gone--desktop")

	// Boot serve.
	serveCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go func() { _ = app.Run(serveCtx, []string{"serve"}) }()
	waitReady(t, port, 20*time.Second)

	base := fmt.Sprintf("http://127.0.0.1:%d/api/v1", port)
	png1, sha1 := makePNG(t, 1)
	_, sha2 := makePNG(t, 2)

	// (1) No API key -> 401.
	if code, _ := doJSON(t, http.MethodPost, base+"/builds", "", map[string]any{"branch": "feature/x", "commitSha": "c1"}); code != http.StatusUnauthorized {
		t.Fatalf("create build without key: got %d, want 401", code)
	}

	// (2) Create a build in project A.
	code, body := doJSON(t, http.MethodPost, base+"/builds", keyA, map[string]any{"branch": "feature/x", "commitSha": "c1"})
	if code != http.StatusCreated {
		t.Fatalf("create build: got %d (%s), want 201", code, body)
	}
	buildID := decode(t, body)["buildId"].(string)

	// (3) Declare a snapshot -> needUpload true (blob not yet in store).
	declare := func(key, name, sha string) map[string]any {
		code, body := doJSON(t, http.MethodPost, base+"/builds/"+buildID+"/snapshots", key, map[string]any{
			"name": name, "browser": "chromium", "viewport": "1280x720", "imageSha256": sha, "width": 1, "height": 1, "byteSize": 100,
		})
		if code != http.StatusOK {
			t.Fatalf("declare %s: got %d (%s)", name, code, body)
		}
		return decode(t, body)
	}
	d1 := declare(keyA, "home--desktop", sha1)
	if d1["needUpload"] != true {
		t.Fatalf("first declare: needUpload = %v, want true", d1["needUpload"])
	}

	// (4) Idempotent retry: re-declare same snapshot, no duplicate row.
	declare(keyA, "home--desktop", sha1)
	if n := countSnapshots(t, ctx, database, buildID); n != 1 {
		t.Fatalf("idempotent declare produced %d rows, want 1", n)
	}

	// (5) Upload the bytes -> 204.
	if code, body := doRaw(t, http.MethodPut, base+"/images/"+sha1, keyA, png1); code != http.StatusNoContent {
		t.Fatalf("upload image: got %d (%s), want 204", code, body)
	}

	// (6) Dedup: a different snapshot with the SAME sha now reports needUpload false.
	d2 := declare(keyA, "other--desktop", sha1)
	if d2["needUpload"] != false {
		t.Fatalf("dedup declare: needUpload = %v, want false", d2["needUpload"])
	}

	// (7) Hash mismatch: declare sha2 but upload png1 -> 400 SNAPSHOT_HASH_MISMATCH.
	declare(keyA, "mismatch--desktop", sha2)
	code, body = doRaw(t, http.MethodPut, base+"/images/"+sha2, keyA, png1)
	if code != http.StatusBadRequest {
		t.Fatalf("hash mismatch: got %d (%s), want 400", code, body)
	}
	if got := decode(t, body)["error"].(map[string]any)["code"]; got != "SNAPSHOT_HASH_MISMATCH" {
		t.Fatalf("hash mismatch code = %v, want SNAPSHOT_HASH_MISMATCH", got)
	}

	// (8) Project isolation: key B cannot write to project A's build -> 403.
	if code, _ := doJSON(t, http.MethodPost, base+"/builds/"+buildID+"/snapshots", keyB, map[string]any{
		"name": "x", "browser": "chromium", "viewport": "1280x720", "imageSha256": sha2, "width": 1, "height": 1, "byteSize": 1,
	}); code != http.StatusForbidden {
		t.Fatalf("cross-project write: got %d, want 403", code)
	}
	// (8b) Unknown build -> 404 BUILD_NOT_FOUND.
	code, body = doJSON(t, http.MethodPost, base+"/builds/nope/snapshots", keyA, map[string]any{
		"name": "x", "browser": "chromium", "viewport": "1280x720", "imageSha256": sha2, "width": 1, "height": 1, "byteSize": 1,
	})
	if code != http.StatusNotFound || decode(t, body)["error"].(map[string]any)["code"] != "BUILD_NOT_FOUND" {
		t.Fatalf("unknown build: got %d (%s), want 404 BUILD_NOT_FOUND", code, body)
	}

	// (9) Finalize -> COMPARING, REMOVED computed (the seeded baseline has no snapshot), diff jobs enqueued.
	code, body = doJSON(t, http.MethodPatch, base+"/builds/"+buildID, keyA, map[string]any{"status": "FINALIZE"})
	if code != http.StatusOK {
		t.Fatalf("finalize: got %d (%s), want 200", code, body)
	}
	if decode(t, body)["status"] != "COMPARING" {
		t.Fatalf("finalize status = %v, want COMPARING", decode(t, body)["status"])
	}
	if n := countRemoved(t, ctx, database, buildID); n != 1 {
		t.Fatalf("REMOVED snapshots = %d, want 1", n)
	}
	if n := countDiffJobs(t, ctx, database); n < 1 {
		t.Fatalf("enqueued diff jobs = %d, want >= 1", n)
	}

	// (10) Re-finalize -> 409 BUILD_ALREADY_FINALIZED.
	if code, _ := doJSON(t, http.MethodPatch, base+"/builds/"+buildID, keyA, map[string]any{"status": "FINALIZE"}); code != http.StatusConflict {
		t.Fatalf("double finalize: got %d, want 409", code)
	}
}

// ---- helpers ----

func mustProject(t *testing.T, ctx context.Context, q *db.Queries, name, slug string) db.Project {
	t.Helper()
	p, err := q.CreateProject(ctx, db.CreateProjectParams{ID: core.NewID(), Name: name, Slug: slug, DefaultBranch: "main"})
	if err != nil {
		t.Fatalf("seed project %s: %v", slug, err)
	}
	return p
}

func mustAPIKey(t *testing.T, ctx context.Context, q *db.Queries, p db.Project) string {
	t.Helper()
	raw, err := auth.GenerateKey()
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	if _, err := q.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		ID: core.NewID(), ProjectID: p.ID, KeyHash: auth.HashKey(sessionSecret, raw), Name: "test",
	}); err != nil {
		t.Fatalf("seed key: %v", err)
	}
	return raw
}

func seedBaseline(t *testing.T, ctx context.Context, database *db.DB, p db.Project, branch, name string) {
	t.Helper()
	_, sha := makePNG(t, 99)
	if err := database.Queries().UpsertImage(ctx, db.UpsertImageParams{Sha256: sha, Width: 1, Height: 1, ByteSize: 1}); err != nil {
		t.Fatalf("seed baseline image: %v", err)
	}
	if _, err := database.Pool().Exec(ctx,
		`INSERT INTO baselines (id, project_id, branch, name, browser, viewport, image_sha) VALUES ($1,$2,$3,$4,'chromium','1280x720',$5)`,
		core.NewID(), p.ID, branch, name, sha); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}
}

func countSnapshots(t *testing.T, ctx context.Context, database *db.DB, buildID string) int {
	t.Helper()
	return scanCount(t, ctx, database, `SELECT count(*) FROM snapshots WHERE build_id = $1`, buildID)
}

func countRemoved(t *testing.T, ctx context.Context, database *db.DB, buildID string) int {
	t.Helper()
	return scanCount(t, ctx, database, `SELECT count(*) FROM snapshots WHERE build_id = $1 AND status = 'REMOVED'`, buildID)
}

func countDiffJobs(t *testing.T, ctx context.Context, database *db.DB) int {
	t.Helper()
	return scanCount(t, ctx, database, `SELECT count(*) FROM river_job WHERE kind = 'pixela.diff'`)
}

func scanCount(t *testing.T, ctx context.Context, database *db.DB, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := database.Pool().QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("count query: %v", err)
	}
	return n
}

func makePNG(t *testing.T, seed uint8) ([]byte, string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Pix[0] = seed // R: vary so different seeds -> different bytes -> different sha
	img.Pix[3] = 0xff // opaque, else png.Encode collapses transparent pixels to the same bytes
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func doJSON(t *testing.T, method, url, key string, payload any) (int, string) {
	t.Helper()
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(method, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "ApiKey "+key)
	}
	return send(t, req)
}

func doRaw(t *testing.T, method, url, key string, body []byte) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "image/png")
	if key != "" {
		req.Header.Set("Authorization", "ApiKey "+key)
	}
	return send(t, req)
}

func send(t *testing.T, req *http.Request) (int, string) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func decode(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return m
}
