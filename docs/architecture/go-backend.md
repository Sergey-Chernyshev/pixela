# Pixela — Go Backend Architecture & Conventions

> **Status:** Committed rulebook for the Go backend rewrite (API + async diff worker). Supersedes the NestJS/Prisma/BullMQ stack described in the current `CLAUDE.md` and `docs/spec/specs/02-architecture.md` *for the backend only* — the Playwright reporter, the Angular dashboard, and the data model / API contract specs remain authoritative. The architectural **invariants** in `CLAUDE.md` (git-native baseline, blobs-in-S3, stateless ingestion + async diff, CAS, project isolation, determinism > features) are unchanged and binding.
>
> **Audience:** the 7-8 person team. This is opinionated on purpose. When a research source offered a menu, we picked one and you adhere to it.

---

## 0. Scale & guiding principle

Pixela is **explicitly not hyperscale**: ~50 active projects, ~500 screenshots/build, history over months, self-hosted via docker-compose. Every decision below optimizes for **clarity, determinism, and a small team's velocity** — not throughput. The recurring failure mode to avoid is *over-engineering in the name of a convention*. Diff is the cheapest part; do not overcomplicate it.

---

## 1. The stack (one line)

Go 1.25 single binary (subcommands `serve|worker|migrate`) · **Huma v2 on chi** (code-first OpenAPI 3.1 → `openapi-typescript`) · **pgx/v5 + sqlc + Atlas** (Postgres) · **River** (Postgres-transactional queue) · **pure-Go pixelmatch + `image/png`** diff (`CGO_ENABLED=0`) · **log/slog + caarlos0/env** · **golangci-lint v2 + Taskfile + air** · **distroless/static** · **Testcontainers**.

### 1.1 Resolved cross-cutting decisions (where the research disagreed)

These are the conflicts between research outputs, decided once:

1. **Router = Huma v2 on chi**, *not* bare `net/http` 1.22 `ServeMux`. The brief mandates code-first OpenAPI emission; Huma is the engine for that. Huma is router-agnostic and sits on `net/http`, so the "1.22 routing is enough" point is **subsumed**, not contradicted — chi provides the infra middleware layer, Huma provides typed operations + spec emission. We do not also hand-roll a `ServeMux`.
2. **Process modes = explicit subcommands** `pixela serve | worker | migrate`, *not* the spec's `API_MODE=http|worker` env flag. Subcommands are self-documenting and testable. This is a **deliberate change** from `02-architecture.md` and Phase-0; docker-compose simply sets `command: [pixela, serve]` / `command: [pixela, worker]`. Use the stdlib `flag` package — **not Cobra** (the Huma spec-emit example uses Cobra, but a 3-way `os.Args[1]` switch needs no dependency).
3. **Redis stays — for dashboard sessions only.** The queue research said "no Redis in the stack." That overreaches. River removes Redis *as a queue*, but the spec mandates **server-side dashboard sessions in Redis** (`SESSION_SECRET`, decision B-03) and Phase-0 `/readyz` pings it. Redis is **out of the data/queue path, in for sessions**. (If you later prefer Postgres-backed sessions to drop Redis entirely, that is a separate, allowed optimization — but not assumed here.)
4. **Config = caarlos0/env v11**, not bare `os.Getenv` and not viper/koanf. Strictly 12-factor, validated once at startup.
5. **Health = split `/healthz` + `/readyz`**, replacing Phase-0's single `/health`. Liveness is dependency-free; readiness checks Postgres + Redis + MinIO.
6. **DI = hand-wired in `run()`**, no framework. `google/wire` is unmaintained (2024); dig/fx sacrifice compile-time safety.

---

## 2. Project layout

**Authority:** [go.dev/doc/modules/layout](https://go.dev/doc/modules/layout) (official Go team), **not** `golang-standards/project-layout` (community, self-admittedly overkill for this size). Servers export nothing → essentially **all Go code lives under `internal/`** (compiler-enforced privacy, free refactor safety). **No `pkg/`** (pre-`internal` relic). **No `api/build/configs/deployments/scripts/web/` cargo-cult tree.**

The Go backend is one module inside the existing pnpm monorepo, at `apps/api/`:

```
pixela/                              # pnpm-workspace monorepo root (existing)
  apps/api/                          # the Go module (replaces NestJS apps/api)
    go.mod  go.sum                   # toolchain go1.25.x pinned
    cmd/pixela/main.go               # THIN: parse subcommand, call app.Run(ctx, ...)
    internal/
      app/        # composition root: Run(ctx); serve/worker/migrate dispatch; hand-wired DI
      config/     # caarlos0/env Config + validate; Secret type (redacting)
      core/       # dependency-free domain: Build, Snapshot, Baseline, Image, enums, port interfaces
      httpapi/    # NewServer(deps) http.Handler, routes.go, closure handlers, error mapping, encode/decode
        middleware/  # chi-layer: request-id, slog log, CORS, rate-limit, recovery; + Huma auth mw
      auth/       # API-key salted-hash+lookup -> projectId; Redis session; project-isolation guard
      ingestion/  # builds/snapshots, two-phase CAS upload, idempotent upsert, finalize trigger
      diff/       # DiffEngine interface + stdlibEngine{}; DiffOptions/DiffResult; pure, testable
      baseline/   # git-native baseline resolution
      approval/   # approve/reject, ApprovalEvent, build-status recompute (FOR UPDATE)
      notify/     # Telegram / Slack / GitLab MR status (failure-isolated)
      queue/      # River JobArgs+Worker pairs: DiffJob, FinalizeBuildJob; shared by serve & worker
      storage/    # object-store CAS adapter: sha256 keys, presigned URLs, hash-verify
      db/         # pgxpool + sqlc-generated code; ExecTx callback wrapper; imports core only
        sqlc/     # sqlc.yaml + query/*.sql
    db/
      schema.sql                     # SINGLE source of truth (sqlc schema: + Atlas src:)
      migrations/                    # Atlas-generated versioned migrations (committed)
    api/openapi.yaml                 # emitted by `pixela openapi` (go:generate)
    test/                            # testcontainers + goleak + golden-master diff
    .air.toml  .golangci.yml  Taskfile.yml  Dockerfile  .dockerignore
  packages/shared/src/api.d.ts       # openapi-typescript output; reporter + Angular consume
  packages/sdk/                      # @pixela/playwright-reporter (TS, unchanged)
  apps/web/                          # Angular dashboard (TS, unchanged)
  docker-compose.dev.yml             # postgres:16 + minio + redis:7(sessions) + traefik
```

**Dependency DAG (must never cycle — Go enforces this at compile time):**
`cmd → app → {httpapi, queue, ingestion, diff, baseline, approval, notify, storage, db, auth} → core`. `core` imports nothing of ours. Adapters (`db`, `storage`, `queue`, `httpapi`, `notify`) depend inward only; they meet **only** in `app`.

**Package naming:** name for the **capability provided** (`ingestion`, `diff`, `baseline`, `storage`, `notify`, `httpapi`). **Never** `util`, `common`, `helper`, `models`, `base` — the Google Style Guide flags these as dumping grounds. **Never** leave `Build`/`Snapshot`/`Baseline` inside `db` (the "misplaced domain model" anti-pattern); they live in `core`.

**Breaking an import cycle:** extract a *narrow* interface at the **consumer (leaf)** and move it to the lowest package needed. Do not restructure.

---

## 3. Architecture style: lightweight hexagonal

Use the **one** load-bearing idea of hexagonal — *dependencies point inward, the domain depends on nothing* — and **stop there.**

- **Do:** a dependency-free `internal/core` (types + status enums + small port interfaces); feature/service packages with business logic; thin adapters at the edges.
- **Do not:** the full clean-architecture ceremony — 4 concentric layers, `ports/`+`adapters/` literal dirs, per-layer DTOs + mapper structs, CQRS, DDD aggregates/value-objects, repository-per-entity explosion. That pays off at large-team / long-horizon scale; here it buys indirection and mapper boilerplate, no testability you don't already get from constructor injection.

### 3.1 Dependency injection — by hand, in `run()`

No DI framework. Every component is a struct from a `New…` constructor that takes its dependencies as arguments and returns `*Struct` (or `http.Handler`). **Accept interfaces, return structs** — but only define an interface where a consumer *genuinely* needs to swap/mock (storage, queue, notifier, clock, DiffEngine), and keep them **tiny (1-3 methods)**. Do not split an interface for a *single* implementation just to obey the rule.

The composition root is `internal/app.Run(ctx, ...)`, called from `main`, wiring in this order:

```
config (caarlos0/env, validate) → pgxpool (Ping) → object-store client →
River client → domain services → httpapi.NewServer / queue.Workers
```

The **same constructed dependencies** feed both the `serve` and `worker` paths.

---

## 4. Language do / don't rules (Go 1.22-1.25)

These are drawn from Effective Go, Go Code Review Comments, and the Google Go Style Guide. They are binding.

**Adopt (modern features):**
- `net/http` 1.22+ method+wildcard routing semantics (used *via* Huma/chi; understand `r.PathValue`).
- `log/slog` (§9).
- `errors.Join` / multi-`%w`, `errors.Is`/`As` (§5).
- `min`/`max`/`clear` and the `slices`/`maps` packages — delete every hand-rolled `Contains`/`Map`/sort loop.
- Generics **only with a real unifying constraint**: `encode[T]`/`decode[T]` JSON helpers, a paginated `list[T]` response wrapper. *Write code, don't design types* — if one concrete type is instantiated, don't make it generic.
- `iter.Seq`/`Seq2` **only** for genuinely lazy/streaming sequences (e.g. streaming `pgx.Rows`); name collection iterators `All()`; do **not** convert slice-returning APIs reflexively.
- Go 1.24 **tool directives in `go.mod`** for `sqlc`/`atlas`/`river` CLIs (drop `tools.go`).
- Go 1.25 **container-aware `GOMAXPROCS`** (don't hardcode it; it self-tunes to the container CPU limit).
- `signal.NotifyContext` for shutdown; context flows through every layer.

**Avoid (anti-patterns):**
- **GORM / any reflection ORM** — conflicts with the determinism + schema-as-contract invariants. Use sqlc typed queries.
- **DI frameworks** (wire unmaintained; dig/fx reflection).
- **Third-party HTTP router as the base** (chi is fine as the infra layer under Huma; do not also add gin/echo/gorilla).
- **`panic` for control flow across package boundaries.** Errors are values, returned. `recover` only at two places: the HTTP top middleware and **every worker goroutine** (see §6).
- **Global mutable state / package-level vars** for the DB pool, config, or logger — pass through constructors.
- **`init()` with side effects, I/O, failure, or ordering deps.** `init` may only set pure data; if package init truly must die, `panic` is the only acceptable death — **never `log.Fatal` in `init`**.
- **Naked returns** in non-trivial functions (and don't name results just to enable them).
- **Interface pollution** — don't pre-wrap stores/clients in interfaces "for testing"; define interfaces in the *consumer* with only the methods used.
- **`util`/`common`/`models`/`helper` packages.**
- Keep the **happy path unindented**: handle errors with early `return`, drop the unnecessary `else`.

---

## 5. Error handling

- **Errors are values**, wrapped with context: `fmt.Errorf("verb noun: %w", err)` — category early, lowercase, no trailing punctuation.
- **`%w` is an API contract.** Use it only when callers should programmatically inspect the cause; use `%v` to deliberately hide an implementation detail (e.g. don't leak pgx internals across a package boundary).
- **Sentinels are always returned wrapped.** `var ErrSnapshotNotFound = errors.New("snapshot not found")`; return `fmt.Errorf("%s: %w", id, ErrSnapshotNotFound)`. Callers use `errors.Is`, never `==`. Returning a bare sentinel freezes your API on `==`.
- **Inspect with `errors.Is` / `errors.As`** — never string-match `err.Error()`. Critically: `errors.As` onto `*pgconn.PgError` to detect `23505` unique-violation and route it to the idempotent-upsert / `SNAPSHOT_HASH_MISMATCH` handling.
- **Aggregate** per-snapshot validation failures with `errors.Join`.
- **Map domain errors → HTTP status / API error codes in ONE place** at the handler edge (`SNAPSHOT_HASH_MISMATCH`, `IMAGE_TOO_LARGE`, `BUILD_NOT_FOUND`, `BUILD_ALREADY_FINALIZED`, `FORBIDDEN_PROJECT`, `VALIDATION_ERROR`, `UNAUTHORIZED`), matching the spec's unified `{ "error": { "code", "message" } }` envelope.
- **Banned:** `github.com/pkg/errors` (superseded by stdlib `%w`/`Is`/`As`/`Join`); `panic`-based propagation across boundaries; logging *and* returning the same error (double-handling) — log once at the top frame.

---

## 6. Concurrency & the diff worker

**Prefer synchronous APIs; let the caller add concurrency.** Make every goroutine's exit evident.

- **Job-level concurrency is River's, not yours.** Set `QueueConfig{MaxWorkers: runtime.NumCPU()}` (pixelmatch + PNG decode are CPU-bound). **Do not** add a second goroutine-per-job pool inside the worker — that double-counts concurrency and oversubscribes CPU, breaking diff determinism. Horizontal scale = `docker-compose scale pixela-worker=N` → `N × NumCPU` diffs; River's `SKIP LOCKED` fetch hands disjoint jobs to each. Backpressure is automatic (unfetched jobs stay `available` in Postgres).
- **Inner fan-out** (e.g. decoding baseline + candidate + masks in parallel inside one job): bound it with `errgroup.WithContext` + `g.SetLimit(N)`. Pass the **derived `gctx`** (not the parent `ctx`) into each `g.Go` so first-error cancels siblings. Always `g.Wait()`.
- **errgroup does NOT recover panics and does NOT collect all errors** (only the first). So **wrap every `g.Go` body and every River worker `Work` body with a deferred `recover()` that converts panic → error.** This is exactly spec §07's "падение одного diff-job не валит билд" — a corrupt PNG must not crash the worker process.
- **Every blocking goroutine selects on `ctx.Done()`** so `StopAndCancel` can interrupt it; River cancels via context and cannot kill goroutines that ignore it.
- **Always `defer cancel()`** on any `WithCancel`/`WithTimeout`/`WithDeadline`. Set per-operation deadlines on S3 and Postgres calls so a hung dependency can't leak a worker.

**Graceful shutdown ladder (worker):** SIGTERM → `Stop(ctx)` (drain in-flight) → second signal / timeout → `StopAndCancel(ctx)` (cancel contexts, still persist results) → exit. Await the `Stopped()` channel.

---

## 7. HTTP + OpenAPI → TS contract flow

### 7.1 Server structure (Mat Ryer, Grafana 2024)

- `NewServer(deps...) http.Handler` builds the chi mux + Huma API and delegates to **one `routes.go`** (`addRoutes`) that lists the *entire* API surface.
- **Handlers are closures / standalone funcs taking deps** — `func handleApproveSnapshot(deps) http.Handler`. **NOT** methods on a `Server` struct (Ryer explicitly dropped that in 2024: hides deps, brittle tests).
- Thin `main` → `run(ctx, ...)`; generic `encode[T]`/`decode[T]` JSON helpers.

### 7.2 Code-first OpenAPI

- Define request/response **Go structs**, register operations with `huma.Register`. Huma reflects them into **OpenAPI 3.1 + JSON Schema** and validates inputs at the edge, returning **RFC 9457** structured errors via `huma.WriteErr`.
- **Validation lives in struct tags** (`required`, `minimum`, `format`, `enum`, `path:`/`query:`/`header:` binding) — the spec and the runtime check derive from the *same* struct (replaces class-validator). Size limit → `IMAGE_TOO_LARGE`; invalid → `VALIDATION_ERROR`.
- Map the API contract (`04-api-contract.md`) under `/api/v1`: `POST /builds`, `POST /builds/:id/snapshots` (two-phase declare-then-`PUT /images/:sha256`), `PATCH /builds/:id` (finalize), the dashboard reads, `approve`/`reject`, `approve-all`/`reject-all`, auth, key management. SSE (`/builds/:id/events`) is a plain chi handler (streaming, not a Huma operation).

### 7.3 Auth (drive enforcement off declared OpenAPI Security)

- Register two security schemes in `config.Components.SecuritySchemes`: **`ApiKeyAuth`** (apiKey-in-header, `Authorization: ApiKey <key>`) for CI ingestion, **`SessionCookie`** (apiKey-in-cookie) for the dashboard. Per operation: `Security: {{ "ApiKeyAuth": {} }}` or `{{ "SessionCookie": {} }}`.
- **One Huma middleware** loops over `ctx.Operation().Security`, enforces only the declared scheme(s), hashes/looks up the API key → `projectId` (or validates the Redis session), and stashes the principal via `huma.WithValue`. **Docs == enforcement** — adding a scheme both documents and turns on the guard. No per-handler ad-hoc auth checks.
- **API-key hashing:** salted **fast hash** (SHA-256+salt) — high-entropy keys don't need bcrypt (spec §10). Reserve **argon2id** for user passwords.
- **Project isolation** is enforced at the **query level** (`WHERE project_id IN (user's projects)`), not just in code filtering — the response must never include data from inaccessible projects.

### 7.4 Infra middleware (chi layer, before routing)

request-id, slog JSON access log, **CORS restricted to the dashboard origin** (spec §10), **rate-limit** (per-project for ingestion, per-IP/email for login), panic-recovery. These are connection-level concerns — **not** inside Huma operation middleware.

### 7.5 Spec emission & the drift gate (single source of truth = Go handler structs)

1. A `pixela openapi` subcommand calls `api.OpenAPI().YAML()` and prints the spec **without booting the server**: `//go:generate go run ./cmd/pixela openapi > api/openapi.yaml`.
2. `openapi-typescript` reads `api/openapi.yaml` → `packages/shared/src/api.d.ts`. Both the Playwright reporter and Angular consume it via `openapi-fetch` `createClient<paths>({ baseUrl })` with `{ data, error }`.
3. **CI drift gate:** regenerate `openapi.yaml`, re-run `openapi-typescript`, then `git diff --exit-code api/openapi.yaml packages/shared/src/api.d.ts` (no native `--check` flag exists yet — this is the standard gate). Add `tsc --noEmit` so a contract break surfaces in the TS consumers. Drift becomes **structurally impossible to merge**.

### 7.6 Lifecycle

`ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM); defer stop()`. Start `http.Server{Handler: NewServer(...)}` in a goroutine; on `<-ctx.Done()` call `srv.Shutdown(freshTimeoutCtx)` with a **fresh** `context.WithTimeout(~30s)` — **never the cancelled signal/errgroup ctx** (that drops in-flight connections → 502 bursts). Treat `http.ErrServerClosed` as clean. Run the HTTP server and any in-process components under `errgroup.WithContext`.

---

## 8. Data layer & migrations

### 8.1 sqlc + pgx/v5 + pgxpool (no ORM)

- **sqlc v1.31** (`sql_package: pgx/v5`) generates typed Go from plain SQL. Single `db/schema.sql` is the source of truth. Generated code is **committed and diffable**. `sqlc.yaml`: `emit_json_tags`, `emit_db_tags`, `emit_pointers_for_null_types`, `emit_enum_valid_method`, `emit_all_enum_values`.
- **pgxpool**, one pool per process, `pgxpool.ParseConfig` → `NewWithConfig` → `pool.Ping(ctx)` immediately (the pool returns before any connection is established). Conservative sizing: `MaxConns ~10-25`, `MinConns ~2-5`, `MaxConnLifetime 1h` + `MaxConnLifetimeJitter ~5m`, `MaxConnIdleTime 30m`, `HealthCheckPeriod 1m`, explicit `PingTimeout`. Expose `pool.Stat()` on `/readyz`.
- **Banned:** `lib/pq` + `database/sql` (lib/pq is in maintenance mode; sqlc targets pgx/v5 natively).

### 8.2 Enums

Native `CREATE TYPE … AS ENUM` for `BuildStatus`, `SnapshotStatus`, `ApprovalAction`, `Role` — sqlc emits an aliased string type + typed constants, mirroring the Prisma enums 1:1. Keep enum columns **`NOT NULL DEFAULT`** (e.g. `status build_status NOT NULL DEFAULT 'RUNNING'`) to match the spec's `@default(...)` and to sidestep sqlc bug #3969 (nullable enums regress to `interface{}`).

### 8.3 The two queries that matter

**Idempotent snapshot upsert** (CI-retry safe, per spec). Declare `CONSTRAINT snapshot_build_name_browser_viewport_key UNIQUE (build_id, name, browser, viewport)` and:

```sql
-- name: UpsertSnapshot :one
INSERT INTO snapshots (id, build_id, name, browser, viewport, new_image_sha, status)
VALUES (@id, @build_id, @name, @browser, @viewport, @new_image_sha, 'PENDING')
ON CONFLICT (build_id, name, browser, viewport)
DO UPDATE SET new_image_sha = EXCLUDED.new_image_sha, status = 'PENDING'
RETURNING *;
```

IDs are **app-generated cuid2 `TEXT` primary keys** passed as `@id` (matching Prisma `@default(cuid())`) — **not** uuid/serial.

**Atomic build-status recompute** (concurrent workers finalize the same build): a pgx transaction via an `ExecTx` callback wrapper + sqlc `WithTx`, `SELECT id FROM builds WHERE id=@id FOR UPDATE` (Read Committed) to serialize, then one aggregate:

```sql
SELECT count(*) FILTER (WHERE status IN ('CHANGED','NEW','ERROR')) AS needs_review,
       count(*) FILTER (WHERE status='PENDING')                    AS pending
FROM snapshots WHERE build_id=@id;
```

compute the new `BuildStatus` per the state machine (one `ERROR` → `REVIEW_REQUIRED`, never whole-build `ERROR`), `UPDATE … finalized_at=now()`. **`FOR UPDATE` removes the race without a Serializable+`40001` retry loop.** Note: **pgx does NOT auto-rollback on context cancel** (unlike `database/sql`) — that is why the `BeginTxFunc`/`ExecTx` callback wrapper is mandatory. The Store holds `DBTX` (not the concrete pool) so the same generated queries run against pool or tx; **do not** store the tx in context.

### 8.4 Migrations — Atlas

Atlas in **declarative mode** reading the *same* `db/schema.sql` as desired state, with **versioned authoring**: `atlas migrate diff <name> --to file://db/schema.sql --dev-url 'docker://postgres/16/dev'` materializes a reviewable, committed migration in `db/migrations/`; apply with `atlas migrate apply --url "$DATABASE_URL"`. **`atlas migrate lint` gates CI** — DS103 destructive-drop fails by default; data-dependent changes (NOT NULL on a non-empty table, nullable→non-null, risky `ALTER TYPE … ADD VALUE`) warn/fail — turning the spec's "migrations irreversible in prod, change with care" into an **enforced gate**. **Banned:** golang-migrate (unrecoverable "dirty" state needs privileged prod access; no lint); Atlas pure-`schema apply` straight to prod (no reviewed artifact); hand-written down-migrations.

> **Phase-0 deviation (intentional, documented).** Atlas is the **authoring + CI-lint authority** from day one (the schema source of truth is `db/schema.sql`; `atlas migrate diff`/`migrate lint` own versioning and safety). But `pixela migrate` in Phase 0 applies the **embedded** initial schema (`db/schema.sql` via `embed`, in a single transaction, idempotent via a sentinel-table guard — no "dirty" state) rather than shelling out to the `atlas` binary at runtime. Rationale: it keeps the serve/worker/migrate binary a single `CGO_ENABLED=0` distroless/static image with **no `atlas` runtime dependency**, and the initial schema is exactly one version. **Versioned Atlas migrations (generated artifacts + `atlas migrate apply`, or `atlasexec`) take over from the first schema change (Phase 1+)** — at which point a deploy-time migration step (the `arigaio/atlas` image, or `atlasexec`) applies them. This is the one sanctioned exception to "no hand-rolled apply"; it is *initial-schema bootstrap*, not a general migrator.

---

## 9. Queue & workers — River

- **River** (`riverqueue/river` + `riverpgxv5`) on the **same Postgres + pgx pool**. **Why over asynq:** the diff job is enqueued with **`InsertTx` inside the same transaction as the snapshot upsert** — "jobs are enqueued iff their tx commits, removed on rollback, invisible until commit." This eliminates the entire lost-job (snapshot stuck `PENDING`) / phantom-job class that a separate Redis queue reintroduces, with zero extra code. Use **`InsertManyTx`** (Postgres `COPY FROM`) to batch a build's ~500 snapshots.
- **serve** = insert-only client (omit `Queues`, don't call `Start`). **worker** = full client with `Workers` registered + `Queues{default:{MaxWorkers: runtime.NumCPU()}}` + `Start`. Both register the same `Workers` bundle so River validates every job kind has a worker.
- **Unique jobs** (partial unique index): `DiffJob` unique `ByArgs` on `snapshotId`, `FinalizeBuildJob` unique `ByArgs` on `buildId` — at-least-once-safe dedup of CI retries.
- **Crash recovery is free:** River's rescuer (`RescueAfter`) + `river_leader` LISTEN/NOTIFY leader election re-pick-up jobs stuck `running` after a worker crash — exactly the spec's "незавершённые jobs переподхватываются".
- **Build finalization = a separate `FinalizeBuildJob` kind**, not "the last diff job finalizes inline." Each diff job, in its own tx, updates its snapshot and checks remaining-non-terminal count; the one that observes `0` enqueues `FinalizeBuildJob` via `InsertTx` (unique by `buildId`). Finalize computes the aggregate `Build.status` + `REMOVED` detection **exactly once**, with its own retry policy, isolating a notify/finalize failure from diff failures (spec: a notification failure must not break build processing).
- **Ops dashboard:** `riverui` behind Pixela's auth (needs only `DATABASE_URL`). Don't pin the broken `v0.12.0`.

---

## 10. Image / diff engine & determinism seam

### 10.1 Engine: pure Go for v1

stdlib `image/png` decode+encode + **`orisano/pixelmatch`** (zero non-stdlib deps, pinned by exact commit) — maps 1:1 onto the spec's pixelmatch algorithm. **`CGO_ENABLED=0`, one static binary.** **Do not** pull in govips/bimg/libvips now: determinism is the hard invariant and **libvips 8.18.0 (Dec 2025) changed its default PNG backend ("prefer libpng over spng"), changing output bytes** — fatal for a sha256-content-addressed store; govips itself documents segfaults/leaks. libvips stays behind a build-tagged `DiffEngine`, **worker-only**, enabled only if profiling proves stdlib decode is the bottleneck on >4MP PNGs — and even then used **only for decode → canonical NRGBA**, never to encode the stored PNG.

### 10.2 The seam (consumer-side interface, in `diff/`)

```go
type DiffEngine interface {
    Decode(r io.Reader) (image.Image, error)
    Diff(baseline, candidate image.Image, opts DiffOptions) (DiffResult, error)
}
// DiffOptions{PixelThreshold float64, IncludeAA bool, DiffColor color.Color, IgnoreRects []image.Rectangle}
// DiffResult{Status, DiffPixels int, DiffRatio float64, DiffImage image.Image}
```

Operate on `image.Image`, **not `[]byte`** — keeps the engine independent of codec choice and enables decoded-pixel addressing. **Encoding the diff PNG + computing the content-address are a SEPARATE seam** (Encoder/CAS), so the diff engine never owns byte serialization. Default impl is a concrete `stdlibEngine{}` **injected as a dependency** (Mat Ryer style) — the API binary builds without ever importing the diff impl; tests inject a fake. Phase 0 ships only the interface + a stub + wiring; the real engine lands in Phase 2.

### 10.3 Content addressing — by canonical decoded pixels

`Image.sha256` / S3 key = `sha256` over a **fixed serialization of decoded pixels normalized to NRGBA / 8-bit / straight (non-premultiplied) alpha / no gamma / no ICC**. For the **diff image specifically**, namespace the key: `sha256("pixela-diff/v1" || width || height || canonical_pixels)`. **Verify uploads** by re-decoding + re-hashing pixels (`SNAPSHOT_HASH_MISMATCH`), never by trusting the byte stream. **Why not hash compressed bytes:** the avalanche property — any toolchain/encoder/`CompressionLevel` change silently re-keys identical content and breaks CAS dedup; the version tag lets you migrate the diff renderer intentionally instead of discovering a flake.

### 10.4 Determinism controls (lock all, regardless of engine)

1. Pin the Go toolchain via the `go.mod` `toolchain` directive + build with `-trimpath` (`image/png` bytes depend on `compress/flate`, which shifts between Go versions).
2. Reuse one `png.Encoder{CompressionLevel: png.BestCompression}` value for every diff PNG — never `DefaultCompression`.
3. Pin `orisano/pixelmatch` by exact commit (it's stable but unmaintained).
4. Choose `includeAA`, `pixelThreshold` default, and diff color **once**; assert them in tests.
5. Normalize every decode to NRGBA/8-bit/straight-alpha/no-gamma **before both diffing and hashing**.
6. Apply ignore-rects by **deterministically zeroing pixels in BOTH buffers** before pixelmatch.
7. **Golden-master test:** a fixed PNG pair → assert exact `diffPixels`, `diffRatio`, **and** the diff-image content hash, in CI. This is what turns "we pinned things" into a guarantee.

**Disable** all optional PNG decode transforms (gamma, ICC/color-management, 16→8 scaling, alpha compositing/premultiply) — they are the documented sources of cross-decoder pixel divergence. **Size mismatch = `CHANGED` with `diffRatio=1.0`** (spec MVP decision (a)), do not "smart-resize." **No downscale-before-diff** unless an explicit per-project opt-in (it changes sensitivity).

---

## 11. Observability & config

### 11.1 Logging — log/slog only

- `slog.NewJSONHandler(os.Stdout, …)` in prod, `NewTextHandler` in dev (switch on `PIXELA_ENV`). `slog.SetDefault(logger)` once in `run()` **and** pass `*slog.Logger` explicitly as a dependency into `NewServer`/services.
- **Request scoping:** middleware generates/propagates `request_id`, derives `logger.With("request_id", id, "method", …, "path", …)`, stores it in context under an **unexported key type**. Handlers retrieve via `LoggerFromContext(ctx)` (falls back to `slog.Default()`, never fails). Wrap the base handler in a `ContextHandler` and use the `InfoContext`/`ErrorContext` variants on hot paths so context attrs auto-appear.
- **Secret hygiene is a hard rule.** Never log `DATABASE_URL`/`REDIS_URL`/`S3_*`/`GITLAB_TOKEN`/`TELEGRAM_BOT_TOKEN`/`SLACK_WEBHOOK_URL`/`SESSION_SECRET`/raw API keys. Implement `slog.HandlerOptions.ReplaceAttr` redacting known sensitive keys to `[REDACTED]`, and make secret config fields a `Secret string` type whose `LogValue()`/`String()` returns `[REDACTED]`. Log API keys by short prefix only (`pxl_xxxx…`). Log the spec's audit events (key create/revoke, login, approve/reject) with IDs, never secrets.
- **Banned:** zap/zerolog/logrus (unnecessary deps at this scale); storing the `*logger` itself in context as the *primary* mechanism (Go team rejected it as a hidden dependency); logging full request/response payloads.

### 11.2 Config — caarlos0/env v11

One `Config` struct per process, `env.ParseAs[Config]()` in `run()`. `required`/`notEmpty` for must-have secrets (`DATABASE_URL`, `SESSION_SECRET`), `envDefault` for ports/levels, the `file:` tag for Docker/K8s secret mounts. **Validate and fail fast before opening any connection.** The app does **not** read `.env` files itself (anti-12-factor drift); docker-compose / shell injects env; a `direnv`/`make`-loaded `.env` is local-dev-only. Secret fields use the redacting `Secret` type so config can be safely logged at startup.

### 11.3 Health — two probes

- **`/healthz`** (liveness): process-only, dependency-free, instant 200. Never touches DB/Redis/S3.
- **`/readyz`** (readiness): checks Postgres `SELECT 1`, Redis `PING`, MinIO reachability; 200 only if all up, else 503 with the failing dep. Gate with `var ready atomic.Bool` flipped true after migrations/connections; short per-check `context` timeout (~2s); **reuse** the pool/clients, never open per-probe connections.
- Both **unauthenticated, outside `/api`.** docker-compose healthchecks wire to `/readyz`. The **worker** exposes the same pair. This **replaces** Phase-0's single `/health` (which would restart-loop on transient dep blips).

---

## 12. Testing

- **Table-driven + subtests:** `t.Run(tc.name, …)` over a `[]struct`, `t.Parallel()` where safe, `t.Errorf` (non-fatal) so all cases report.
- **Assertions:** stdlib + **`google/go-cmp`** `cmp.Diff(want, got)` (assert on the returned diff string; `cmpopts`/`Transformer` for float `diffRatio`). Use a `-update` flag + **golden files** for binary/complex output (diff-PNG bytes, OpenAPI spec, serialized responses). *Pragmatic:* `testify/require` is acceptable if the team prefers it, but pick **one** assertion style repo-wide; the default here is stdlib + `cmp.Diff`.
- **Integration:** **Testcontainers** (`postgres:16` + `minio`) applying **real Atlas migrations** before running sqlc queries (catches drift between `schema.sql`, generated Go, and real Postgres behavior — e.g. the unique-constraint/`ON CONFLICT` semantics ingestion depends on). Guard with `SkipIfProviderIsNotHealthy` so they skip cleanly without Docker; `defer testcontainers.TerminateContainer`; `TestMain` for setup + migrations. This matches the committed Phase-0 "Smoke на Testcontainers" decision.
- **Race + leaks:** `go test -race` in CI; `go.uber.org/goleak` (`VerifyTestMain`) to enforce the no-goroutine-leak requirement in the worker; Go 1.24 `testing/synctest` for retry/backoff timing tests.
- **Banned:** mocking the DB for integration paths (misses real Postgres transaction/unique-constraint behavior); 100% coverage as a target; mixing assertion styles.

---

## 13. Tooling & CI

- **golangci-lint v2** (pinned version in CI, don't float), `version: "2"`, `linters.default: standard` (errcheck, govet, ineffassign, staticcheck, unused) **+ curated:** `revive`, `gocritic`, `gosec` (auth/secrets service), `bodyclose`, `rowserrcheck` + `sqlclosecheck` (pgx resource leaks), `noctx`, `errorlint`, `contextcheck`, `misspell`, `unconvert`, `unparam`, `nilerr`. `formatters`: `gofmt` + `goimports` (local-prefix). Re-opt-in the human-readable exclusion presets (v2 removed defaults). **Also run `go vet ./...` as a separate step** (bundled govet can lag upstream).
- **Taskfile** (`go-task`) is the command surface, with `includes` per monorepo component: `dev` (air), `build`, `lint`, `vet`, `fmt`, `test` (`-race`), `test:integration`, `migrate`, `generate` (sqlc/openapi), `tidy`, `ci`. **air** for local hot reload (`.air.toml`, rebuilds the single binary in `serve` mode). Makefile only as a 5-line bridge if a CI env guarantees only `make`.
- **CI (GitLab) = separate parallel jobs:** (1) lint (golangci-lint v2 + `go vet`), (2) build, (3) unit (`go test -race -coverprofile`), (4) integration (testcontainers postgres+minio). Testcontainers on GitLab needs **DinD**: `docker:dind` service, `DOCKER_HOST=tcp://docker:2375`, `DOCKER_TLS_CERTDIR=""`, `dockerd --tls=false` (startup-delay fix), `TESTCONTAINERS_HOST_OVERRIDE`. Cache Go module + build cache. The drift gate (§7.5) and `tsc --noEmit` run in CI.
- **Docker:** multi-stage — `golang` builder (`CGO_ENABLED=0 go build -trimpath -ldflags='-s -w'`) → `gcr.io/distroless/static-debian12` (or `scratch` + ca-certificates + `import _ "time/tzdata"`). One image, two modes via `command:`. **Banned:** Alpine/musl for Go; debian-slim as the *default* base; putting libvips in the API image. (If the cgo libvips worker ever ships: split — API stays distroless/static, worker → `distroless/base-debian12` glibc with the **exact pinned libvips version**.)

---

## 14. Phase 0 rewrite scope

The Go backend replaces the NestJS `apps/api`. Phase 0 = the skeleton with clean seams for Phases 1-2 (ingestion / diff / approve), nothing more:

1. **Module skeleton:** `apps/api/go.mod` (pinned `toolchain go1.25.x`), `cmd/pixela/main.go` (thin), `internal/{app,config,core,httpapi,db,auth}` packages, empty-but-typed `internal/{ingestion,diff,baseline,approval,notify,queue,storage}` seams. `internal/app.Run` dispatching `serve|worker|migrate`.
2. **Config:** caarlos0/env `Config` (`DATABASE_URL`, `REDIS_URL`, `S3_*`, `SESSION_SECRET`, `GITLAB_*`, ports, log level), `Secret` redacting type, validate-at-startup. Update `.env.example`.
3. **pgx pool:** `pgxpool` from config, `Ping` on boot, `pool.Stat()` on `/readyz`.
4. **Schema + migrations:** port `03-data-model.md` 1:1 into `db/schema.sql` (8 tables, 4 native enums, the composite `UNIQUE`, the spec indexes, cuid `TEXT` PKs, no `BaselineVersion`). `pixela migrate` applies the embedded initial schema transactionally + creates River's tables (see the §8.4 Phase-0 deviation note: embedded bootstrap now, Atlas versioned migrations from Phase 1). Run `sqlc generate` (committed).
5. **Redis:** client for dashboard sessions (no queue), pinged in `/readyz`.
6. **`/healthz` + `/readyz`:** replace Phase-0's single `/health`; readiness checks Postgres + Redis + MinIO, gated by `atomic.Bool`.
7. **Modes:** `pixela serve` (chi + Huma `NewServer`, insert-only River client) and `pixela worker` (River `Workers` + `Start`) both wire from `internal/app`. River migration tables created.
8. **OpenAPI seam:** `pixela openapi` subcommand emitting `api/openapi.yaml`; `openapi-typescript` → `packages/shared/src/api.d.ts`; CI drift gate + `tsc --noEmit`.
9. **Testcontainers smoke** (keep Phase-0's, ported to Go): clean-DB Atlas migrate + `/readyz` 200; add `goleak`.
10. **Dockerfile** (distroless/static, `CGO_ENABLED=0`, `-trimpath`) + docker-compose updated to the one image / two `command:` modes, Redis retained for sessions.
11. **Taskfile + golangci-lint v2 config + air + `.gitlab-ci.yml`** (lint/build/unit/integration jobs, DinD).

**Out of Phase 0:** the DiffEngine implementation (only the interface + stub + wiring), real ingestion/approve logic, the dashboard endpoints' bodies, notifications. Those land in Phases 1-2 on the seams above.

---

## 15. Sources (authoritative)

- **Layout/idioms:** [go.dev/doc/modules/layout](https://go.dev/doc/modules/layout); [Mat Ryer, "How I write HTTP services after 13 years" (Grafana 2024)](https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/); [Google Go Style Guide](https://google.github.io/styleguide/go/best-practices.html); [Effective Go](https://go.dev/doc/effective_go) / [Code Review Comments](https://go.dev/wiki/CodeReviewComments).
- **Language:** [Routing Enhancements (Go 1.22)](https://go.dev/blog/routing-enhancements); [slog blog](https://go.dev/blog/slog); [go1.13 errors](https://go.dev/blog/go1.13-errors); [range-functions](https://go.dev/blog/range-functions); [errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup); [go1.24](https://go.dev/doc/go1.24).
- **HTTP/OpenAPI:** [Huma](https://github.com/danielgtaylor/huma) / [huma.rocks](https://huma.rocks/features/openapi-generation/) + [spec-cmd example](https://github.com/danielgtaylor/huma/blob/main/examples/spec-cmd/main.go); [openapi-typescript](https://openapi-ts.dev/) / [openapi-fetch](https://openapi-ts.dev/openapi-fetch/) / [no --check flag #1615](https://github.com/openapi-ts/openapi-typescript/issues/1615); [graceful shutdown](https://victoriametrics.com/blog/go-graceful-shutdown/).
- **Data:** [sqlc+pgx](https://docs.sqlc.dev/en/stable/guides/using-go-and-pgx.html) / [enums](https://docs.sqlc.dev/en/stable/reference/datatypes.html) / [#3969](https://github.com/sqlc-dev/sqlc/issues/3969) / [WithTx](https://docs.sqlc.dev/en/latest/howto/transactions.html); [Atlas+sqlc](https://atlasgo.io/guides/frameworks/sqlc-declarative) / [Atlas vs golang-migrate](https://atlasgo.io/blog/2025/04/06/atlas-and-golang-migrate) / [DS103](https://atlasgo.io/guides/mysql/checks/DS103); [pgx v5](https://pkg.go.dev/github.com/jackc/pgx/v5) / pgxpool.
- **Queue:** [River](https://riverqueue.com/) / [docs](https://riverqueue.com/docs) / [unique-jobs](https://riverqueue.com/docs/unique-jobs) / [graceful-shutdown](https://riverqueue.com/docs/graceful-shutdown) / [maintenance](https://riverqueue.com/docs/maintenance-services) / [batch-insert](https://riverqueue.com/docs/batch-job-insertion).
- **Image/determinism:** [libvips ChangeLog](https://raw.githubusercontent.com/libvips/libvips/master/ChangeLog) (8.18 backend swap); [image/png writer](https://go.dev/src/image/png/writer.go); [orisano/pixelmatch](https://github.com/orisano/pixelmatch); [distroless vs scratch](https://oneuptime.com/blog/post/2026-02-08-how-to-choose-between-scratch-and-distroless-base-images/view).
- **Observability/tooling:** [caarlos0/env](https://github.com/caarlos0/env); [golangci-lint v2](https://golangci-lint.run/docs/configuration/file/) / [v2 release](https://ldez.github.io/blog/2025/03/23/golangci-lint-v2/) / [golden config](https://gist.github.com/maratori/47a4d00457a92aa426dbd48a18776322); [k8s probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/); [testcontainers GitLab CI](https://golang.testcontainers.org/system_requirements/ci/gitlab_ci/); [Taskfile](https://taskfile.dev) + [air](https://github.com/air-verse/air); [goleak](https://pkg.go.dev/go.uber.org/goleak).