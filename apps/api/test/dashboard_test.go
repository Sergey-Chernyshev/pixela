//go:build integration

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/minio"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/app"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
)

// TestPhase4Dashboard drives the full dashboard contract against real infra: session login (cookie),
// membership-scoped reads, presigned review URLs, the precise 403-vs-404 distinction, and logout
// revocation. It also exercises the `user create` / `member add` admin CLI used to seed accounts.
func TestPhase4Dashboard(t *testing.T) {
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

	database, err := db.Open(ctx, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(database.Close)
	q := database.Queries()

	projA := mustProject(t, ctx, q, "Project A", "proj-a")
	mustProject(t, ctx, q, "Project B", "proj-b")

	// Seed accounts through the admin CLI (exercises `user create` / `member add` end to end).
	const alicePass = "alice-passw0rd"
	mustCLI(t, ctx, "user", "create", "alice@example.com", alicePass, "Alice")
	mustCLI(t, ctx, "user", "create", "bob@example.com", "bob-passw0rd")
	mustCLI(t, ctx, "member", "add", "alice@example.com", "proj-a", "OWNER")
	// bob is intentionally NOT a member of proj-a (used for the 403 cases).

	buildID, changedSnap, newSnap := seedReviewBuild(t, ctx, database, projA)

	serveCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go func() { _ = app.Run(serveCtx, []string{"serve"}) }()
	waitReady(t, port, 20*time.Second)

	base := fmt.Sprintf("http://127.0.0.1:%d/api/v1", port)

	// (1) Wrong password and (2) unknown email both yield the same generic 401 INVALID_CREDENTIALS.
	for _, cred := range []struct{ email, pass string }{
		{"alice@example.com", "nope"},
		{"ghost@example.com", "whatever"},
	} {
		_, code, body := login(t, base, cred.email, cred.pass)
		if code != http.StatusUnauthorized || errCode(t, body) != "INVALID_CREDENTIALS" {
			t.Fatalf("login %q/%q: got %d (%s), want 401 INVALID_CREDENTIALS", cred.email, cred.pass, code, body)
		}
	}

	// (3) Unauthenticated dashboard read -> 401.
	if code, body := getJSON(t, http.DefaultClient, base+"/auth/me"); code != http.StatusUnauthorized {
		t.Fatalf("me without cookie: got %d (%s), want 401", code, body)
	}

	// (4) Login as alice -> 200 + Set-Cookie; the jar client now carries the session.
	alice, code, body := login(t, base, "alice@example.com", alicePass)
	if code != http.StatusOK {
		t.Fatalf("alice login: got %d (%s), want 200", code, body)
	}
	if obj(t, body)["email"] != "alice@example.com" {
		t.Fatalf("login body email = %v, want alice@example.com", obj(t, body)["email"])
	}

	// (5) /auth/me reflects the session.
	code, body = getJSON(t, alice, base+"/auth/me")
	if code != http.StatusOK || obj(t, body)["email"] != "alice@example.com" {
		t.Fatalf("me: got %d (%s), want 200 alice", code, body)
	}

	// (6) /projects is membership-scoped: alice sees proj-a (role OWNER), never proj-b.
	code, body = getJSON(t, alice, base+"/projects")
	if code != http.StatusOK {
		t.Fatalf("projects: got %d (%s)", code, body)
	}
	projects := arr(t, obj(t, body)["projects"])
	if len(projects) != 1 {
		t.Fatalf("alice projects = %d, want 1 (proj-a only)", len(projects))
	}
	if p0 := projects[0].(map[string]any); p0["slug"] != "proj-a" || p0["role"] != "OWNER" {
		t.Fatalf("project[0] = %v, want slug proj-a role OWNER", p0)
	}

	// (7) Build feed: one build, per-status snapshot counts computed in SQL.
	code, body = getJSON(t, alice, base+"/projects/"+projA.ID+"/builds")
	if code != http.StatusOK {
		t.Fatalf("builds: got %d (%s)", code, body)
	}
	feed := obj(t, body)
	items := arr(t, feed["items"])
	if len(items) != 1 || numEq(feed["totalPages"], 1) == false {
		t.Fatalf("build feed items=%d totalPages=%v, want 1 / 1 (%s)", len(items), feed["totalPages"], body)
	}
	counts := items[0].(map[string]any)["counts"].(map[string]any)
	if !numEq(counts["changed"], 1) || !numEq(counts["new"], 1) || !numEq(counts["unchanged"], 1) || !numEq(counts["removed"], 1) {
		t.Fatalf("counts = %v, want changed1/new1/unchanged1/removed1", counts)
	}

	// (7b) An out-of-range page is clamped to [1, totalPages] — never a 500, and Page <= TotalPages.
	code, body = getJSON(t, alice, base+"/projects/"+projA.ID+"/builds?page=999")
	if code != http.StatusOK {
		t.Fatalf("out-of-range page: got %d (%s), want 200 (clamped, not 500)", code, body)
	}
	if oob := obj(t, body); !numEq(oob["page"], 1) || !numEq(oob["totalPages"], 1) {
		t.Fatalf("out-of-range page: page=%v totalPages=%v, want page<=totalPages (1/1)", oob["page"], oob["totalPages"])
	}

	// (8) Build detail lists all snapshots.
	code, body = getJSON(t, alice, base+"/builds/"+buildID)
	if code != http.StatusOK {
		t.Fatalf("build detail: got %d (%s)", code, body)
	}
	if snaps := arr(t, obj(t, body)["snapshots"]); len(snaps) != 4 {
		t.Fatalf("build detail snapshots = %d, want 4", len(snaps))
	}

	// (9) Snapshot review: presigned URLs for baseline/new/diff + approval history.
	code, body = getJSON(t, alice, base+"/snapshots/"+changedSnap)
	if code != http.StatusOK {
		t.Fatalf("snapshot review: got %d (%s)", code, body)
	}
	review := obj(t, body)
	if review["status"] != "CHANGED" {
		t.Fatalf("review status = %v, want CHANGED", review["status"])
	}
	images := review["images"].(map[string]any)
	for _, k := range []string{"baseline", "new", "diff"} {
		if images[k] == nil || images[k] == "" {
			t.Fatalf("review images[%q] is empty, want a presigned URL (%s)", k, body)
		}
	}
	if hist := arr(t, review["history"]); len(hist) != 1 || hist[0].(map[string]any)["action"] != "APPROVE" {
		t.Fatalf("review history = %v, want one APPROVE by alice", review["history"])
	}

	// (10) A NEW snapshot (no baseline) has a null baseline URL but a non-null new URL.
	code, body = getJSON(t, alice, base+"/snapshots/"+newSnap)
	if code != http.StatusOK {
		t.Fatalf("new-snapshot review: got %d (%s)", code, body)
	}
	if img := obj(t, body)["images"].(map[string]any); img["baseline"] != nil {
		t.Fatalf("NEW snapshot baseline = %v, want null", img["baseline"])
	}

	// (11) Precise 404s for unknown ids (still authenticated as alice).
	if code, body := getJSON(t, alice, base+"/builds/does-not-exist"); code != http.StatusNotFound || errCode(t, body) != "BUILD_NOT_FOUND" {
		t.Fatalf("unknown build: got %d (%s), want 404 BUILD_NOT_FOUND", code, body)
	}
	if code, body := getJSON(t, alice, base+"/snapshots/does-not-exist"); code != http.StatusNotFound || errCode(t, body) != "NOT_FOUND" {
		t.Fatalf("unknown snapshot: got %d (%s), want 404 NOT_FOUND", code, body)
	}

	// (12) Non-member (bob) gets 403 on every proj-a resource — never 404 (no existence leak beyond 403).
	bob, code, _ := login(t, base, "bob@example.com", "bob-passw0rd")
	if code != http.StatusOK {
		t.Fatalf("bob login: got %d, want 200", code)
	}
	for _, url := range []string{
		base + "/projects/" + projA.ID + "/builds",
		base + "/builds/" + buildID,
		base + "/snapshots/" + changedSnap,
	} {
		if code, body := getJSON(t, bob, url); code != http.StatusForbidden || errCode(t, body) != "FORBIDDEN_PROJECT" {
			t.Fatalf("bob GET %s: got %d (%s), want 403 FORBIDDEN_PROJECT", url, code, body)
		}
	}

	// (13) Logout revokes the session: the same cookie no longer authenticates.
	if code, body := postJSON(t, alice, base+"/auth/logout", nil); code != http.StatusOK {
		t.Fatalf("logout: got %d (%s), want 200", code, body)
	}
	if code, _ := getJSON(t, alice, base+"/auth/me"); code != http.StatusUnauthorized {
		t.Fatalf("me after logout: got %d, want 401", code)
	}
}

// ---- dashboard-specific helpers ----

// login posts credentials and returns a cookie-jar client carrying any session set by the response.
func login(t *testing.T, base, email, password string) (*http.Client, int, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	client := &http.Client{Jar: jar}
	code, body := postJSON(t, client, base+"/auth/login", map[string]any{"email": email, "password": password})
	return client, code, body
}

func getJSON(t *testing.T, client *http.Client, url string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	return doReq(t, client, req)
}

func postJSON(t *testing.T, client *http.Client, url string, payload any) (int, string) {
	t.Helper()
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return doReq(t, client, req)
}

func doReq(t *testing.T, client *http.Client, req *http.Request) (int, string) {
	t.Helper()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// mustCLI runs an admin subcommand in-process and fails the test on error.
func mustCLI(t *testing.T, ctx context.Context, args ...string) {
	t.Helper()
	if err := app.Run(ctx, args); err != nil {
		t.Fatalf("cli %v: %v", args, err)
	}
}

// seedReviewBuild inserts one finalized build in proj on its default branch with four snapshots —
// CHANGED (baseline+new+diff, with an APPROVE event), NEW (new only), UNCHANGED, and REMOVED — so the
// dashboard reads have every status to render. Returns the build id and the CHANGED/NEW snapshot ids.
func seedReviewBuild(t *testing.T, ctx context.Context, database *db.DB, proj db.Project) (buildID, changedSnap, newSnap string) {
	t.Helper()
	q := database.Queries()
	pool := database.Pool()

	// Content blobs (only the rows need to exist; presign signs a URL regardless of object presence).
	_, baseSha := makePNG(t, 10)
	_, newSha := makePNG(t, 11)
	_, diffSha := makePNG(t, 12)
	_, newSha2 := makePNG(t, 13)
	for _, sha := range []string{baseSha, newSha, diffSha, newSha2} {
		if err := q.UpsertImage(ctx, db.UpsertImageParams{Sha256: sha, Width: 1, Height: 1, ByteSize: 1}); err != nil {
			t.Fatalf("seed image: %v", err)
		}
	}

	baseID := core.NewID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO baselines (id, project_id, branch, name, browser, viewport, image_sha)
		 VALUES ($1,$2,$3,'home--desktop','chromium','1280x720',$4)`,
		baseID, proj.ID, proj.DefaultBranch, baseSha); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}

	buildID = core.NewID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO builds (id, project_id, branch, commit_sha, ci_job_url, status)
		 VALUES ($1,$2,$3,'abc123def','https://ci.example/job/1','REVIEW_REQUIRED')`,
		buildID, proj.ID, proj.DefaultBranch); err != nil {
		t.Fatalf("seed build: %v", err)
	}

	changedSnap = core.NewID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO snapshots (id, build_id, name, browser, viewport, new_image_sha, diff_image_sha, baseline_id, diff_ratio, diff_pixels, status)
		 VALUES ($1,$2,'home--desktop','chromium','1280x720',$3,$4,$5,0.12,345,'CHANGED')`,
		changedSnap, buildID, newSha, diffSha, baseID); err != nil {
		t.Fatalf("seed changed snapshot: %v", err)
	}
	newSnap = core.NewID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO snapshots (id, build_id, name, browser, viewport, new_image_sha, status)
		 VALUES ($1,$2,'about--desktop','chromium','1280x720',$3,'NEW')`,
		newSnap, buildID, newSha2); err != nil {
		t.Fatalf("seed new snapshot: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO snapshots (id, build_id, name, browser, viewport, new_image_sha, baseline_id, status)
		 VALUES ($1,$2,'pricing--desktop','chromium','1280x720',$3,$4,'UNCHANGED')`,
		core.NewID(), buildID, newSha, baseID); err != nil {
		t.Fatalf("seed unchanged snapshot: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO snapshots (id, build_id, name, browser, viewport, baseline_id, status)
		 VALUES ($1,$2,'contact--desktop','chromium','1280x720',$3,'REMOVED')`,
		core.NewID(), buildID, baseID); err != nil {
		t.Fatalf("seed removed snapshot: %v", err)
	}

	// An approval event on the CHANGED snapshot (audit history), authored by alice.
	alice, err := q.GetUserByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("get alice: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO approval_events (id, snapshot_id, user_id, action) VALUES ($1,$2,$3,'APPROVE')`,
		core.NewID(), changedSnap, alice.ID); err != nil {
		t.Fatalf("seed approval event: %v", err)
	}
	return buildID, changedSnap, newSnap
}

// ---- tiny JSON assertion helpers ----

func obj(t *testing.T, body string) map[string]any { return decode(t, body) }

func arr(t *testing.T, v any) []any {
	t.Helper()
	a, ok := v.([]any)
	if !ok {
		t.Fatalf("value %v is not a JSON array", v)
	}
	return a
}

func errCode(t *testing.T, body string) string {
	t.Helper()
	e, ok := decode(t, body)["error"].(map[string]any)
	if !ok {
		return ""
	}
	code, _ := e["code"].(string)
	return code
}

// numEq compares a JSON number (decoded as float64) to an int.
func numEq(v any, want int) bool {
	f, ok := v.(float64)
	return ok && int(f) == want
}
