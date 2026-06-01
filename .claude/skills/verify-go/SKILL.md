---
name: verify-go
description: >-
  Verify Go backend code with independent review agents right after writing or changing it, in the
  Pixela monorepo (apps/api). Use this whenever you have just written, generated, refactored, or
  edited Go code — handlers, services, sqlc/db, queue workers, config, middleware — BEFORE committing
  or claiming the work is done. It runs a deterministic gate (gofmt · go vet · golangci-lint · build ·
  test -race) and then fans out adversarial reviewers against the project's Go conventions
  (docs/architecture/go-backend.md). Also trigger when the user says "review/verify/check my Go",
  "run the Go review", "is this idiomatic", "did I leak a goroutine", or before opening a PR that
  touches Go. Prefer this over an ad-hoc eyeball pass — the whole point is that fresh, independent
  agents catch what the author (you) is blind to.
---

# verify-go — post-write Go verification for Pixela

Writing code and judging code are different jobs, and the author is the worst judge of their own
fresh code. This skill makes the judging step **independent**: cheap deterministic tooling first, then
several **separate review agents** that each hold one concern in focus and read the same rulebook —
so violations of the Pixela Go conventions surface before they reach a commit or a teammate.

**Rulebook (authoritative):** `docs/architecture/go-backend.md`. The condensed, per-dimension review
checklist this skill drives is in `references/review-dimensions.md` — read it before reviewing, and
hand the relevant slice to each agent. These same rules are how you should *write* Go here in the
first place; verification just enforces them.

## When to run

Run after any non-trivial Go change in `apps/api` and before `git commit` / PR. Skip it for a pure
doc/comment edit or a one-line obvious fix — but if you touched control flow, errors, concurrency,
SQL, auth, or the diff/image path, run it.

## The pipeline

Do these in order. **Do not** spend review agents on what the compiler and linters already catch.

### 1. Scope the change

Determine exactly which Go files changed — that is the review surface.

```bash
cd apps/api
git diff --name-only --diff-filter=ACMR -- '*.go' ; git diff --cached --name-only --diff-filter=ACMR -- '*.go'
```

If you just wrote the files in this session and they aren't committed, use that list. Keep the set
tight; reviewers do better on a focused diff than on the whole tree.

### 2. Deterministic gate (must be green before agents)

Run `scripts/gate.sh` (from the skill dir) — it runs, from `apps/api`: `gofmt -l`, `go vet ./...`,
`golangci-lint run` (per `.golangci.yml`), `go build ./...`, and `go test -race ./...`. Fix anything
it reports **first** — it is faster and more certain than any agent, and a red gate makes agent review
noisy. The gate is also exactly what CI runs, so a green gate locally means a green pipeline.

If `golangci-lint` is missing, install the pinned version (`task tools:install` or
`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@<pinned>`); don't silently skip it.

### 3. Fan out independent reviewers (the core of the skill)

Spawn the reviewers as **parallel subagents** — one per dimension — using the Agent tool (or the
Workflow tool if available; `scripts/review-workflow.js` is a ready orchestration you can run/adapt).
Give each agent ONLY: its dimension's checklist slice from `references/review-dimensions.md`, the
changed files, and a pointer to `docs/architecture/go-backend.md`. Isolation is the point — a focused
agent with one concern outperforms one agent juggling all of them.

Select dimensions by what the diff touched (don't run all seven blindly):

| Dimension | Run when the diff touches… |
| --- | --- |
| `idioms` — layout, naming, DI, happy-path, generics/iterators | always |
| `errors` — `%w` contract, wrapped sentinels, `errors.Is/As`, pgconn 23505, no double-handling | any error handling |
| `concurrency` — errgroup `gctx`, `defer recover` in workers + top mw, `ctx.Done()`, `defer cancel`, no second pool | goroutines, River workers, errgroup |
| `data` — idempotent `ON CONFLICT`, `FOR UPDATE` recompute, `ExecTx` (pgx no auto-rollback), enums `NOT NULL DEFAULT`, no ORM | sqlc/db, queries, migrations |
| `security` — no secrets in logs (`Secret`/`ReplaceAttr`), API-key hashing, project isolation at query level, auth off declared OpenAPI security | auth, logging, handlers, config |
| `http` — Mat Ryer `NewServer`/closure handlers (not struct methods), Huma struct validation, error→status in one place, graceful shutdown fresh ctx, OpenAPI drift gate | httpapi, routes, handlers |
| `determinism` — decoded-pixel addressing, pinned toolchain/pixelmatch, single png encoder, golden test | diff/, image, storage CAS |

Each reviewer returns findings as `{title, severity (blocker|high|medium|low|nit), file, line, rule (cite the go-backend.md section), evidence, fix}`. An empty list is a good result — instruct them not to invent issues.

### 4. Adversarially verify each finding

A plausible-sounding finding that's actually wrong wastes everyone's time. For each finding, have an
independent agent (default verdict: **refuted**) re-read the cited file and confirm it only if the
evidence truly holds and it genuinely violates a stated rule (not personal taste, not a later-phase
gap). Drop the rest. For thorough reviews, use 2-3 verifiers per finding and keep majority-confirmed.

### 5. Report and act

Report confirmed findings sorted by severity, each as `file:line — rule — what & fix`. Then:

- **blocker / high** → fix now (or, if it's a judgment call, surface it to the user with the trade-off).
- **medium / low / nit** → fix if cheap, otherwise list them so they're not silently dropped.

State plainly what the gate and the review found. If everything is clean, say so with the evidence
(gate green, N dimensions reviewed, 0 confirmed) — don't hedge.

## Notes

- This skill **reviews**; it does not replace tests. New non-trivial logic still needs table-driven
  tests + (for diff/CAS) golden files, per the rulebook §12.
- Keep reviewer prompts lean and rule-anchored. If a dimension keeps producing noise, the fix is a
  sharper checklist slice in `references/review-dimensions.md`, not more agents.
