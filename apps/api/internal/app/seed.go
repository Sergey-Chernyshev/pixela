package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/auth"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/config"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/db"
	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/storage"
)

// Demo credentials printed at the end of `pixela seed-demo`.
const (
	demoEmail    = "demo@pixela.dev"
	demoPassword = "pixela-demo" //nolint:gosec // demo seed password, printed to the operator; not a secret
	demoProject  = "acme-storefront"
)

// runSeedDemo populates a fresh database + object store with a realistic demo dataset so the dashboard
// is non-empty out of the box (one command → log in → see populated screens). It is idempotent: if the
// demo project already exists it prints the credentials and exits without touching anything.
//
// Everything it writes is real data through the same tables the ingestion path uses — PNG bytes land in
// MinIO content-addressed by sha256, metadata in Postgres. Nothing is faked at the API layer.
func runSeedDemo(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	database, err := db.Open(ctx, cfg.DatabaseURL.Reveal(), log)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer database.Close()
	q := database.Queries()
	pool := database.Pool()

	store, err := storage.New(ctx, storage.Config{
		Endpoint: cfg.S3Endpoint, PublicEndpoint: cfg.S3PublicEndpoint, Region: cfg.S3Region,
		Bucket: cfg.S3Bucket, AccessKey: cfg.S3AccessKey.Reveal(), SecretKey: cfg.S3SecretKey.Reveal(),
		UseSSL: cfg.S3UseSSL,
	}, log)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}

	// Idempotency: bail out cleanly if the demo project is already present.
	if proj, err := q.GetProjectBySlug(ctx, demoProject); err == nil {
		fmt.Printf("demo already seeded (project %q, id %s)\n", proj.Slug, proj.ID)
		printDemoCreds("")
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check existing project: %w", err)
	}

	// ---- user ----
	user, err := q.GetUserByEmail(ctx, demoEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		hash, herr := auth.HashPassword(demoPassword)
		if herr != nil {
			return fmt.Errorf("hash password: %w", herr)
		}
		name := "Demo Reviewer"
		user, err = q.CreateUser(ctx, db.CreateUserParams{ID: core.NewID(), Email: demoEmail, Name: &name, PasswordHash: &hash})
	}
	if err != nil {
		return fmt.Errorf("seed user: %w", err)
	}

	// A second user so the team/activity screens have more than one actor.
	bot := mustSeedUser(ctx, q, "ci@pixela.dev", "CI Bot")

	// ---- project + membership + api key ----
	proj, err := q.CreateProject(ctx, db.CreateProjectParams{ID: core.NewID(), Name: "Storefront", Slug: demoProject, DefaultBranch: "main"})
	if err != nil {
		return fmt.Errorf("seed project: %w", err)
	}
	for _, m := range []struct {
		uid  string
		role db.Role
	}{{user.ID, db.RoleOWNER}, {bot.ID, db.RoleMEMBER}} {
		if _, err := q.CreateMembership(ctx, db.CreateMembershipParams{ID: core.NewID(), UserID: m.uid, ProjectID: proj.ID, Role: m.role}); err != nil {
			return fmt.Errorf("seed membership: %w", err)
		}
	}
	rawKey, err := auth.GenerateKey()
	if err != nil {
		return err
	}
	if _, err := q.CreateAPIKey(ctx, db.CreateAPIKeyParams{ID: core.NewID(), ProjectID: proj.ID, KeyHash: auth.HashKey(cfg.SessionSecret.Reveal(), rawKey), Name: "ci"}); err != nil {
		return fmt.Errorf("seed api key: %w", err)
	}

	// ---- images (real PNG bytes → MinIO, content-addressed) ----
	put := func(c color.NRGBA, header bool) string {
		shaHex, perr := putImage(ctx, store, q, pagePNG(c, header))
		if perr != nil {
			err = perr
		}
		return shaHex
	}
	baselineSha := put(color.NRGBA{0x2b, 0x37, 0x55, 0xff}, true) // indigo-ish page
	newChanged := put(color.NRGBA{0x2b, 0x55, 0x3b, 0xff}, true)  // greenish page (the change)
	diffSha := put(color.NRGBA{0xff, 0x28, 0xaa, 0xff}, false)    // magenta diff band
	newOnlySha := put(color.NRGBA{0x55, 0x45, 0x2b, 0xff}, true)  // a brand-new page
	unchangedSha := put(color.NRGBA{0x33, 0x33, 0x3b, 0xff}, true)
	if err != nil {
		return fmt.Errorf("seed images: %w", err)
	}

	// ---- baselines (branch main) ----
	type blSpec struct{ name, sha string }
	baselines := map[string]string{} // name -> baseline id
	for _, b := range []blSpec{
		{"home--desktop", baselineSha},
		{"footer--desktop", unchangedSha},
		{"search-overlay--desktop", baselineSha},
	} {
		id := core.NewID()
		if _, err := pool.Exec(ctx,
			`INSERT INTO baselines (id, project_id, branch, name, browser, viewport, image_sha, approved_by_user_id)
			 VALUES ($1,$2,'main',$3,'chromium','1280x720',$4,$5)`,
			id, proj.ID, b.name, b.sha, user.ID); err != nil {
			return fmt.Errorf("seed baseline %s: %w", b.name, err)
		}
		baselines[b.name] = id
	}

	// ---- build 1: feature branch under review (every status represented) ----
	ratio := 0.0347
	pixels := int32(29336)
	build1 := core.NewID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO builds (id, project_id, branch, commit_sha, ci_job_url, status, created_at, finalized_at)
		 VALUES ($1,$2,'feat/checkout-redesign','a1b9f3c','https://gitlab.example/job/1487','REVIEW_REQUIRED',
		         now() - interval '12 minutes', now() - interval '10 minutes')`,
		build1, proj.ID); err != nil {
		return fmt.Errorf("seed build1: %w", err)
	}
	changedSnap := core.NewID()
	type snapSpec struct {
		id, name, status      string
		newSha, diffSha, blID *string
		ratio                 *float64
		pixels                *int32
	}
	str := func(s string) *string { return &s }
	b1Snaps := []snapSpec{
		{changedSnap, "home--desktop", "CHANGED", str(newChanged), str(diffSha), str(baselines["home--desktop"]), &ratio, &pixels},
		{core.NewID(), "promo-banner--desktop", "NEW", str(newOnlySha), nil, nil, nil, nil},
		{core.NewID(), "footer--desktop", "UNCHANGED", str(unchangedSha), nil, str(baselines["footer--desktop"]), nil, nil},
		{core.NewID(), "search-overlay--desktop", "REMOVED", nil, nil, str(baselines["search-overlay--desktop"]), nil, nil},
	}
	for _, s := range b1Snaps {
		if _, err := pool.Exec(ctx,
			`INSERT INTO snapshots (id, build_id, name, browser, viewport, new_image_sha, diff_image_sha, baseline_id, diff_ratio, diff_pixels, status)
			 VALUES ($1,$2,$3,'chromium','1280x720',$4,$5,$6,$7,$8,$9)`,
			s.id, build1, s.name, s.newSha, s.diffSha, s.blID, s.ratio, s.pixels, s.status); err != nil {
			return fmt.Errorf("seed snapshot %s: %w", s.name, err)
		}
	}

	// ---- build 2: green run on main ----
	build2 := core.NewID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO builds (id, project_id, branch, commit_sha, ci_job_url, status, created_at, finalized_at)
		 VALUES ($1,$2,'main','e7c2d10','https://gitlab.example/job/1490','PASSED',
		         now() - interval '3 hours', now() - interval '3 hours' + interval '118 seconds')`,
		build2, proj.ID); err != nil {
		return fmt.Errorf("seed build2: %w", err)
	}
	for _, name := range []string{"home--desktop", "footer--desktop"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO snapshots (id, build_id, name, browser, viewport, new_image_sha, baseline_id, status)
			 VALUES ($1,$2,$3,'chromium','1280x720',$4,$5,'UNCHANGED')`,
			core.NewID(), build2, name, unchangedSha, baselines[name]); err != nil {
			return fmt.Errorf("seed build2 snapshot %s: %w", name, err)
		}
	}

	// ---- activity: a couple of approval events ----
	for _, ev := range []struct {
		uid, action string
		ago         string
	}{{user.ID, "APPROVE", "2 days"}, {bot.ID, "REJECT", "5 days"}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO approval_events (id, snapshot_id, user_id, action, created_at)
			 VALUES ($1,$2,$3,$4, now() - $5::interval)`,
			core.NewID(), changedSnap, ev.uid, ev.action, ev.ago); err != nil {
			return fmt.Errorf("seed approval event: %w", err)
		}
	}

	fmt.Printf("seeded demo project %q with 2 builds, 6 snapshots, 3 baselines, 2 events.\n", proj.Slug)
	printDemoCreds(rawKey)
	return nil
}

// mustSeedUser creates (or fetches) a user by email; panics-free, returns the row.
func mustSeedUser(ctx context.Context, q *db.Queries, email, name string) db.User {
	if u, err := q.GetUserByEmail(ctx, email); err == nil {
		return u
	}
	hash, _ := auth.HashPassword(core.NewID())
	n := name
	u, _ := q.CreateUser(ctx, db.CreateUserParams{ID: core.NewID(), Email: email, Name: &n, PasswordHash: &hash})
	return u
}

// putImage stores PNG bytes in the object store under their sha256 and upserts the image metadata row.
func putImage(ctx context.Context, store *storage.Store, q *db.Queries, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	shaHex := hex.EncodeToString(sum[:])
	if err := store.Put(ctx, shaHex, data); err != nil {
		return "", fmt.Errorf("put image: %w", err)
	}
	cfg, _ := png.DecodeConfig(bytes.NewReader(data))
	//nolint:gosec // demo image dimensions/size are small and bounded
	if err := q.UpsertImage(ctx, db.UpsertImageParams{Sha256: shaHex, Width: int32(cfg.Width), Height: int32(cfg.Height), ByteSize: int32(len(data))}); err != nil {
		return "", fmt.Errorf("upsert image: %w", err)
	}
	return shaHex, nil
}

// pagePNG renders a small, recognizable "page" PNG: a solid body with an optional darker header strip,
// so demo thumbnails and the review viewer show something page-like rather than a flat swatch.
func pagePNG(c color.NRGBA, header bool) []byte {
	const w, h = 480, 320
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px := c
			if header && y < 48 {
				px = color.NRGBA{c.R / 2, c.G / 2, c.B / 2, 0xff} // header strip
			}
			img.SetNRGBA(x, y, px)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func printDemoCreds(apiKey string) {
	fmt.Printf("\n  Dashboard: log in with\n    email:    %s\n    password: %s\n", demoEmail, demoPassword)
	if apiKey != "" {
		fmt.Printf("  Reporter API key (project %q, shown once):\n    %s\n", demoProject, apiKey)
	}
}
