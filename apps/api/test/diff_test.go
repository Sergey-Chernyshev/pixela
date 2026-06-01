//go:build integration

package test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"testing"
	"time"

	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/minio"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/app"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/diff"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/ingestion"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/queue"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/storage"
)

// TestPhase2DiffPipeline drives the full Agent-04 diff pipeline against real infra: it ingests one
// build whose snapshots exercise every terminal verdict (UNCHANGED / CHANGED / NEW / ERROR / REMOVED),
// runs the real `pixela worker` River consumer end-to-end, and asserts the per-snapshot statuses, the
// CHANGED snapshot's diff fields + stored diff blob, the REVIEW_REQUIRED aggregate build status, and
// diff-engine determinism. Diff runs ONLY in the worker (invariant #3), never in this test's threads.
func TestPhase2DiffPipeline(t *testing.T) {
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	ctx := context.Background()

	// Dependencies start SEQUENTIALLY — concurrent first-time starts race on Docker port publishing.
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

	// 1. migrate (schema + River tables).
	if err := app.Run(ctx, []string{"migrate"}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	database, err := db.Open(ctx, dsn, log)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(database.Close)
	q := database.Queries()

	// 2. Seed a project + key, then construct the ingestion service directly and drive it in-process.
	proj := mustProject(t, ctx, q, "Phase2 Project", "phase2")
	_ = mustAPIKey(t, ctx, q, proj) // proves the key path works; ingestion is driven via the service.

	store, err := storage.New(ctx, storage.Config{
		Endpoint:  minioEndpoint,
		Region:    "us-east-1",
		Bucket:    "pixela",
		AccessKey: mn.Username,
		SecretKey: mn.Password,
		UseSSL:    false,
	}, log)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	serveQueue, err := queue.NewServeClient(database.Pool(), log)
	if err != nil {
		t.Fatalf("serve queue: %v", err)
	}
	svc := ingestion.NewService(database, store, serveQueue, 5<<20, log)

	const branch = "feature/x"
	build, err := svc.CreateBuild(ctx, proj.ID, ingestion.CreateBuildInput{Branch: branch, CommitSha: "c1"})
	if err != nil {
		t.Fatalf("create build: %v", err)
	}

	// Distinct same-size PNGs (8x8): A vs B differ in pixels (CHANGED); A vs A identical (UNCHANGED).
	pngA, shaA := makeSolidPNG(t, color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff})
	pngB, shaB := makeSolidPNG(t, color.NRGBA{R: 0xF0, G: 0x20, B: 0x30, A: 0xff})

	declareAndUpload := func(name, sha string, data []byte) string {
		t.Helper()
		snapID, _, err := svc.DeclareSnapshot(ctx, proj.ID, build.ID, ingestion.DeclareSnapshotInput{
			Name: name, Browser: "chromium", Viewport: "1280x720",
			ImageSha256: sha, Width: 8, Height: 8, ByteSize: int32(len(data)),
		})
		if err != nil {
			t.Fatalf("declare %s: %v", name, err)
		}
		if err := svc.UploadImage(ctx, sha, data); err != nil {
			t.Fatalf("upload %s: %v", name, err)
		}
		return snapID
	}

	// UNCHANGED: baseline image_sha == the uploaded new image's sha (same PNG bytes).
	unchangedID := declareAndUpload("unchanged--desktop", shaA, pngA)
	seedBaselineSha(t, ctx, database, proj.ID, branch, "unchanged--desktop", shaA, 8, 8, len(pngA))

	// CHANGED: baseline is a DIFFERENT image (B) than the snapshot's new image (A); both blobs exist.
	changedID := declareAndUpload("changed--desktop", shaA, pngA)
	if err := store.Put(ctx, shaB, pngB); err != nil { // ensure baseline blob exists in the store
		t.Fatalf("put baseline blob: %v", err)
	}
	seedBaselineSha(t, ctx, database, proj.ID, branch, "changed--desktop", shaB, 8, 8, len(pngB))

	// NEW: no baseline at all for this key.
	newID := declareAndUpload("new--desktop", shaB, pngB)

	// ERROR: a snapshot whose declared blob has a valid PNG magic header but is NOT decodable. We bypass
	// UploadImage's sha/decode validation by seeding the blob + image row + snapshot row directly. It
	// needs a baseline (a valid decodable blob) so the worker reaches the decode of the corrupt NEW image
	// and fails there → ERROR, rather than short-circuiting to NEW for a missing baseline.
	errorID := seedCorruptSnapshot(t, ctx, database, store, build.ID, "error--desktop")
	seedBaselineSha(t, ctx, database, proj.ID, branch, "error--desktop", shaB, 8, 8, len(pngB))

	// REMOVED: a baseline whose (name,browser,viewport) has NO snapshot in the build → finalize creates
	// a REMOVED snapshot.
	_, removedBaseSha := makeSolidPNG(t, color.NRGBA{R: 0x01, G: 0x02, B: 0x03, A: 0xff})
	seedBaselineSha(t, ctx, database, proj.ID, branch, "removed--desktop", removedBaseSha, 8, 8, 100)

	// 3. Finalize → COMPARING, REMOVED computed, diff jobs enqueued.
	finalized, err := svc.FinalizeBuild(ctx, proj.ID, build.ID)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if finalized.Status != db.BuildStatusCOMPARING {
		t.Fatalf("post-finalize status = %s, want COMPARING", finalized.Status)
	}

	// 4. Run the real worker process. It builds diffrun.Workers + the River client and processes jobs.
	workerCtx, cancelWorker := context.WithCancel(ctx)
	workerErr := make(chan error, 1)
	go func() { workerErr <- app.Run(workerCtx, []string{"worker"}) }()

	// 5. Poll until the build reaches a terminal aggregate status (REVIEW_REQUIRED expected).
	final := waitBuildTerminal(t, ctx, database, build.ID, 30*time.Second)
	if final != db.BuildStatusREVIEWREQUIRED {
		t.Fatalf("aggregate build status = %s, want REVIEW_REQUIRED", final)
	}

	// 6. Assert every per-snapshot verdict.
	assertStatus(t, ctx, q, unchangedID, db.SnapshotStatusUNCHANGED)
	assertStatus(t, ctx, q, changedID, db.SnapshotStatusCHANGED)
	assertStatus(t, ctx, q, newID, db.SnapshotStatusNEW)
	assertStatus(t, ctx, q, errorID, db.SnapshotStatusERROR)

	// REMOVED snapshot was synthesized at finalize and stays REMOVED.
	removed := snapshotByKey(t, ctx, database, build.ID, "removed--desktop")
	if removed.Status != db.SnapshotStatusREMOVED {
		t.Fatalf("removed snapshot status = %s, want REMOVED", removed.Status)
	}

	// CHANGED snapshot must carry diff metrics + a diff blob that actually exists in storage.
	changed, err := q.GetSnapshot(ctx, changedID)
	if err != nil {
		t.Fatalf("reload changed snapshot: %v", err)
	}
	if changed.DiffRatio == nil || *changed.DiffRatio <= 0 {
		t.Fatalf("changed diff_ratio = %v, want > 0", changed.DiffRatio)
	}
	if changed.DiffPixels == nil || *changed.DiffPixels <= 0 {
		t.Fatalf("changed diff_pixels = %v, want > 0", changed.DiffPixels)
	}
	if changed.DiffImageSha == nil {
		t.Fatalf("changed diff_image_sha is nil, want a stored diff blob")
	}
	exists, err := store.Exists(ctx, *changed.DiffImageSha)
	if err != nil {
		t.Fatalf("stat diff blob: %v", err)
	}
	if !exists {
		t.Fatalf("diff blob %s missing in storage", *changed.DiffImageSha)
	}

	// 7. Determinism: the engine that the worker used must yield identical metrics + content-key on a
	// re-run of the SAME two PNGs, and that key must equal the diff_image_sha the pipeline stored.
	engine := diff.NewStdlibEngine()
	baseImg := mustDecode(t, engine, pngB)
	candImg := mustDecode(t, engine, pngA)
	r1, err := engine.Diff(baseImg, candImg, diff.DefaultOptions())
	if err != nil {
		t.Fatalf("diff run 1: %v", err)
	}
	r2, err := engine.Diff(baseImg, candImg, diff.DefaultOptions())
	if err != nil {
		t.Fatalf("diff run 2: %v", err)
	}
	if r1.DiffPixels != r2.DiffPixels || r1.DiffRatio != r2.DiffRatio {
		t.Fatalf("non-deterministic diff: (%d,%v) vs (%d,%v)", r1.DiffPixels, r1.DiffRatio, r2.DiffPixels, r2.DiffRatio)
	}
	key1 := diff.ContentKey(r1.DiffImage)
	key2 := diff.ContentKey(r2.DiffImage)
	if key1 != key2 {
		t.Fatalf("non-deterministic diff content key: %s vs %s", key1, key2)
	}
	if key1 != *changed.DiffImageSha {
		t.Fatalf("pipeline diff_image_sha %s != recomputed content key %s", *changed.DiffImageSha, key1)
	}
	if int32(r1.DiffPixels) != *changed.DiffPixels {
		t.Fatalf("pipeline diff_pixels %d != engine %d", *changed.DiffPixels, r1.DiffPixels)
	}

	// 8. Clean shutdown of the worker.
	cancelWorker()
	select {
	case err := <-workerErr:
		if err != nil {
			t.Fatalf("worker returned error on shutdown: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("worker did not shut down within 15s")
	}
}

// ---- helpers (the Phase-1 ingestion_test.go helpers in this package are reused directly) ----

// makeSolidPNG encodes an 8x8 PNG filled with one opaque color and returns bytes + sha256 of the bytes.
func makeSolidPNG(t *testing.T, c color.NRGBA) ([]byte, string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+0] = c.R
		img.Pix[i+1] = c.G
		img.Pix[i+2] = c.B
		img.Pix[i+3] = c.A
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	data := buf.Bytes()
	return data, sha256Hex(data)
}

// sha256Hex mirrors ingestion's own hashing so a declared sha matches UploadImage's validation.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// seedBaselineSha inserts a baseline pointing at an existing/known image sha (image row upserted too).
func seedBaselineSha(t *testing.T, ctx context.Context, database *db.DB, projectID, branch, name, sha string, w, h, size int) {
	t.Helper()
	if err := database.Queries().UpsertImage(ctx, db.UpsertImageParams{
		Sha256: sha, Width: int32(w), Height: int32(h), ByteSize: int32(size),
	}); err != nil {
		t.Fatalf("seed baseline image: %v", err)
	}
	if _, err := database.Pool().Exec(ctx,
		`INSERT INTO baselines (id, project_id, branch, name, browser, viewport, image_sha)
		 VALUES ($1,$2,$3,$4,'chromium','1280x720',$5)`,
		core.NewID(), projectID, branch, name, sha); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}
}

// seedCorruptSnapshot stores a blob with a valid PNG magic header but undecodable body, registers its
// image + a PENDING snapshot directly (bypassing UploadImage validation), and returns the snapshot id.
func seedCorruptSnapshot(t *testing.T, ctx context.Context, database *db.DB, store *storage.Store, buildID, name string) string {
	t.Helper()
	// PNG signature followed by junk: passes the magic check, fails png.Decode → snapshot ERROR.
	corrupt := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("not-a-real-png-body")...)
	sha := sha256Hex(corrupt)
	if err := store.Put(ctx, sha, corrupt); err != nil {
		t.Fatalf("put corrupt blob: %v", err)
	}
	if err := database.Queries().UpsertImage(ctx, db.UpsertImageParams{Sha256: sha, Width: 8, Height: 8, ByteSize: int32(len(corrupt))}); err != nil {
		t.Fatalf("upsert corrupt image: %v", err)
	}
	snap, err := database.Queries().UpsertSnapshot(ctx, db.UpsertSnapshotParams{
		ID: core.NewID(), BuildID: buildID, Name: name, Browser: "chromium", Viewport: "1280x720", NewImageSha: &sha,
	})
	if err != nil {
		t.Fatalf("upsert corrupt snapshot: %v", err)
	}
	return snap.ID
}

func mustDecode(t *testing.T, engine diff.Engine, data []byte) image.Image {
	t.Helper()
	img, err := engine.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	return img
}

func assertStatus(t *testing.T, ctx context.Context, q *db.Queries, snapshotID string, want db.SnapshotStatus) {
	t.Helper()
	snap, err := q.GetSnapshot(ctx, snapshotID)
	if err != nil {
		t.Fatalf("get snapshot %s: %v", snapshotID, err)
	}
	if snap.Status != want {
		t.Fatalf("snapshot %q status = %s, want %s (err_msg=%v)", snap.Name, snap.Status, want, snap.ErrorMsg)
	}
}

func snapshotByKey(t *testing.T, ctx context.Context, database *db.DB, buildID, name string) db.Snapshot {
	t.Helper()
	var id string
	if err := database.Pool().QueryRow(ctx,
		`SELECT id FROM snapshots WHERE build_id = $1 AND name = $2`, buildID, name).Scan(&id); err != nil {
		t.Fatalf("find snapshot %s: %v", name, err)
	}
	snap, err := database.Queries().GetSnapshot(ctx, id)
	if err != nil {
		t.Fatalf("get snapshot %s: %v", name, err)
	}
	return snap
}

// waitBuildTerminal polls the build row until it leaves COMPARING/RUNNING, returning the terminal status.
func waitBuildTerminal(t *testing.T, ctx context.Context, database *db.DB, buildID string, within time.Duration) db.BuildStatus {
	t.Helper()
	deadline := time.Now().Add(within)
	var last db.BuildStatus
	for time.Now().Before(deadline) {
		b, err := database.Queries().GetBuild(ctx, buildID)
		if err != nil {
			t.Fatalf("poll build: %v", err)
		}
		last = b.Status
		if b.Status != db.BuildStatusRUNNING && b.Status != db.BuildStatusCOMPARING {
			return b.Status
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("build never reached terminal status within %s (last %s)", within, last)
	return last
}
