# ADR 0001 — Backend language: Go (was NestJS/TypeScript)

- **Status:** Accepted (2026-06-01)
- **Deciders:** Sergey Chernyshev (owner)
- **Supersedes (backend only):** the NestJS/Prisma/BullMQ stack in `docs/spec/specs/02-architecture.md`, `docs/spec/backend/tech-decisions.md`, and the Phase-0 NestJS scaffold.
- **Does not change:** the six product invariants in `CLAUDE.md`, the data model (`03-data-model.md`), the API contract (`04-api-contract.md`), the TypeScript Playwright reporter, or the Angular dashboard.

## Context

Phase 0 shipped a working NestJS/TypeScript backend. The owner reconsidered the language. A rigorous, web-verified analysis (5 lenses + judge) concluded that for the spec's *explicitly-not-hyperscale* scale, **TypeScript was the lower-risk default** — chiefly because the Playwright reporter is forced-TS (the `Reporter` API is Node-only) and the Angular dashboard is TS, so a Go backend makes the repo polyglot and trades the zero-codegen `@pixela/shared` single source of truth for an OpenAPI codegen pipeline.

The same analysis recorded the **strongest honest counter**: the owner is fluent in Go and values it, Go is fully capable here (River, sqlc+Atlas, pgx, minio-go, orisano/pixelmatch), and a reasonable architect weighting owner-preference + ops simplicity (single static binary, true in-process diff parallelism) lands on Go. Phase 0 was 266 LOC of plumbing — the cheapest possible moment to switch.

## Decision

**Write the backend (API + async diff worker) in Go.** The owner consciously accepts the polyglot repo and commits to the mitigation kit that makes Go the *right* call rather than a footgun:

1. **Code-first OpenAPI** (Huma v2 on chi) emits OpenAPI 3.1 from Go handler structs → `openapi-typescript` generates `packages/shared/src/api.d.ts` for the reporter + Angular. A **CI drift gate** (`git diff --exit-code` on the emitted spec + generated types, plus `tsc --noEmit`) makes contract drift impossible to merge.
2. **River** (Postgres-transactional queue) instead of a Redis queue — `InsertTx` enqueues the diff job in the *same transaction* as the snapshot row, eliminating the lost-/phantom-job race. Redis is retained **only** for dashboard sessions.
3. **Determinism locked from day one:** content-address by canonically *decoded* pixels (not compressed bytes; diff key tagged `pixela-diff/v1`), pinned Go toolchain + `orisano/pixelmatch`, pure-Go `image/png` (no cgo/libvips in v1 — libvips 8.18 changed its PNG backend bytes), and a golden-master byte-parity test.

The full, binding stack and rules live in **[`docs/architecture/go-backend.md`](../architecture/go-backend.md)**.

### Stack (one line)

Go 1.26 single binary (`serve|worker|migrate`) · Huma v2 on chi · pgx/v5 + sqlc + Atlas · River · pure-Go pixelmatch + `image/png` (`CGO_ENABLED=0`) · log/slog + caarlos0/env · golangci-lint v2 + Taskfile + air · distroless/static · Testcontainers.

## Consequences

**Positive:** owner-fluent codebase; single static binary + trivial docker-compose deploy; true in-process diff parallelism; `InsertTx` removes a whole class of queue races; Atlas `migrate lint` turns "schema is a contract" into an enforced CI gate; determinism hardened (decoded-pixel addressing fixes a latent CAS bug present even in the TS design).

**Negative / accepted costs:** the repo is permanently polyglot (Go backend + TS reporter + TS Angular); the API contract crosses a Go↔TS boundary and must be regenerated via the CI-gated codegen on every change; the Prisma migration DX is replaced by sqlc+Atlas; Phase 0's NestJS scaffold is discarded (~1 day of plumbing).

**Revisit if:** the scale assumption changes (multi-tenant hosted SaaS, thousands of builds/min, routinely >4MP images) — that only strengthens Go further; or if maintaining the codegen gate proves more friction than value, in which case reconsider a TS backend.
