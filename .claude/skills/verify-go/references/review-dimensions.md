# Pixela Go — review dimensions & coding rules

The checklist each reviewer enforces. These are also the rules to **write** by — verification just
catches drift. Each item cites the authoritative section in `docs/architecture/go-backend.md`. Flag a
violation only when the code genuinely breaks a rule (not taste, not a later-phase stub).

Severity guide: **blocker** = breaks an invariant / correctness / leaks secrets or goroutines;
**high** = clear rule violation with real impact; **medium** = rule violation, contained; **low/nit**
= style/consistency.

---

## idioms — layout, naming, DI, modern Go (§2, §3, §4)

- All server code under `internal/`; no `pkg/`; capability-named packages. **No** `util`/`common`/
  `helper`/`models`/`base` dumping grounds. Domain types live in `internal/core`, never in `db`.
- Import DAG points inward and never cycles: `cmd → app → adapters → core`. `core` imports nothing of
  ours. Break a cycle by extracting a *narrow* interface at the consumer, not by restructuring.
- **Accept interfaces, return structs** — but define an interface only where a consumer genuinely needs
  to swap/mock, kept to 1-3 methods. Do **not** wrap a single concrete impl in an interface to obey a rule.
- DI is hand-wired in `app.Run()` via `New…` constructors; **no** DI framework (wire is unmaintained;
  dig/fx are reflection). **No** package-level globals for pool/config/logger — pass them in.
- Happy path is unindented: early `return` on error, drop the unnecessary `else`.
- Adopt modern stdlib: `slices`/`maps`/`min`/`max`/`clear` (delete hand-rolled equivalents); generics
  only with a real unifying constraint; `iter.Seq` only for genuinely lazy sequences (name them `All()`).
- **No** `init()` with side effects/I/O/failure; **no** naked returns in non-trivial funcs; **no** second
  HTTP router (chi is the infra layer under Huma — not gin/echo/gorilla on top).

## errors — values, wrapping, inspection (§5)

- Errors are returned values wrapped with context: `fmt.Errorf("verb noun: %w", err)` — lowercase, no
  trailing punctuation. `%w` only when callers should inspect the cause; `%v` to deliberately hide an
  impl detail across a boundary.
- Sentinels are **always returned wrapped** (`fmt.Errorf("%s: %w", id, ErrX)`); callers use `errors.Is`,
  never `==`. A bare returned sentinel freezes the API on `==`.
- Inspect with `errors.Is`/`errors.As`, **never** `strings.Contains(err.Error(), …)`. Use `errors.As`
  onto `*pgconn.PgError` to detect `23505` (unique violation) → idempotent-upsert / hash-mismatch path.
- Aggregate multi-item failures with `errors.Join`. Map domain errors → HTTP status/codes in **one**
  place at the handler edge (the `{error:{code,message}}` envelope).
- **Banned:** `github.com/pkg/errors`; panic-based propagation across boundaries; logging **and**
  returning the same error (handle once, at the top frame).

## concurrency — goroutine lifecycle, the worker (§6)

- Job concurrency is River's: `MaxWorkers: runtime.NumCPU()`. **Do not** add a second goroutine pool
  inside the worker (oversubscribes CPU, breaks diff determinism).
- Inner fan-out uses `errgroup.WithContext` + `SetLimit(N)`; pass the **derived `gctx`** (not parent
  `ctx`) into each `g.Go`; always `g.Wait()`.
- **errgroup does not recover panics and keeps only the first error.** Every River `Work` body and every
  `g.Go` body has a `defer recover()` converting panic→error (a corrupt PNG must not crash the worker —
  invariant: one failed diff job must not fail the build). The HTTP top middleware also recovers.
- Every blocking goroutine `select`s on `ctx.Done()`. Every `WithCancel/Timeout/Deadline` has
  `defer cancel()`. Set per-op deadlines on S3/Postgres calls. No goroutine without a clear exit.

## data — sqlc, pgx, migrations (§8)

- sqlc-typed queries over pgx/v5 + pgxpool; **no ORM** (GORM/ent). `lib/pq`/`database/sql` banned.
- Idempotent ingestion: `INSERT … ON CONFLICT (build_id,name,browser,viewport) DO UPDATE …` against a
  real `UNIQUE` constraint. App-generated cuid2 `TEXT` PKs (not uuid/serial).
- Atomic build-status recompute under concurrent workers: `ExecTx` wrapper + `SELECT … FOR UPDATE` then
  one aggregate `count(*) FILTER(...)`. **pgx does NOT auto-rollback on ctx cancel** → the `ExecTx`/
  `BeginTxFunc` callback wrapper is mandatory; the Store holds `DBTX`, never stores the tx in context.
- Native `CREATE TYPE … AS ENUM`; enum columns `NOT NULL DEFAULT '…'` (matches `@default`, avoids sqlc
  nullable-enum `interface{}` regression).
- Migrations via Atlas (declarative from the same `db/schema.sql`, versioned committed artifacts);
  `atlas migrate lint` gates destructive changes in CI. **Banned:** golang-migrate, hand-rolled migrators.

## security — secrets, auth, isolation (§7.3, §11.1)

- **Secrets never logged.** Secret config fields are a `Secret` type whose `LogValue()`/`String()` returns
  `[REDACTED]`; `slog` `ReplaceAttr` redacts known sensitive keys. Log API keys by short prefix only.
- API-key auth: salted **fast hash** (SHA-256+salt) lookup → `projectId`. argon2id only for user passwords.
- Auth is driven off the **declared OpenAPI security scheme** in one Huma middleware (docs == enforcement);
  no per-handler ad-hoc auth.
- **Project isolation at the query level** (`WHERE project_id IN (...)`), not just code filtering — a
  response must never include data from inaccessible projects.
- CORS restricted to the dashboard origin; rate-limit ingestion (per-project) and login (per-IP/email).

## http — server shape & contract (§7)

- `NewServer(deps) http.Handler` + one `routes.go` (`addRoutes`) listing the whole surface; handlers are
  **closures/standalone funcs taking deps**, NOT methods on a `Server` struct (hides deps, brittle tests).
- Request/response validation lives in Huma struct tags (`required`,`format`,`enum`,`path:`/`query:`),
  so spec and runtime check derive from the same struct. Errors mapped to the `{error:{code,message}}`
  envelope in one place.
- Graceful shutdown: `srv.Shutdown(ctx)` with a **fresh** `context.WithTimeout` — never the cancelled
  signal/errgroup ctx (drops in-flight conns). Treat `http.ErrServerClosed` as clean.
- Contract: the Go handler structs are the source of truth → `pixela openapi` emits `api/openapi.yaml` →
  `openapi-typescript` → `packages/shared`. A change to a DTO must regenerate both; the CI drift gate
  (`git diff --exit-code` + `tsc --noEmit`) must stay green.

## determinism — diff/CAS (§10) — only when image/diff/storage touched

- Content-address by canonical **decoded pixels** (NRGBA / 8-bit / straight alpha / no gamma / no ICC),
  **not** compressed bytes. Diff-image key namespaced with the encoder tag (`pixela-diff/v1`).
- Pure-Go `image/png` + pinned `orisano/pixelmatch`; **no libvips/cgo in v1**. `DiffEngine` is a
  consumer-side interface operating on `image.Image` (not `[]byte`), injected as a dependency.
- Determinism locks: pinned Go toolchain (`go.mod` `toolchain` + `-trimpath`), one reused
  `png.Encoder{CompressionLevel: BestCompression}`, fixed `includeAA`/threshold/diff-color, decode
  normalization before both diff and hash, ignore-rects zeroed in **both** buffers, **golden-master test**
  asserting exact `diffPixels`/`diffRatio`/diff-image hash. Size mismatch = `CHANGED` ratio 1.0 (no resize).
