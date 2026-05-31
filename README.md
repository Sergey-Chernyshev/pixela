# Pixela — Backend (`pixela-back`)

Self-hosted **visual regression testing** platform. This repo is the backend monorepo:
the NestJS **API/worker** and the Playwright **SDK**. The Angular dashboard lives in a
separate repo, [`pixela-ui`](../pixela-ui).

> Full specification: [`docs/spec/`](docs/spec/). Architectural invariants and working
> agreements: [`CLAUDE.md`](CLAUDE.md). Read those before changing anything.

## Layout

```
apps/api          NestJS app — runs as HTTP API or diff worker (API_MODE=http|worker)
packages/sdk      @pixela/playwright-reporter (scaffold)
packages/shared   shared TypeScript types mirroring the API contract (scaffold)
docs/spec         the canonical product/engineering specification
docker-compose.dev.yml   local infra: postgres + redis + minio
```

## Prerequisites

- **Node 22** (`.nvmrc` → `nvm use`)
- **pnpm 10** (`corepack enable` then `corepack use pnpm@10`)
- **Docker** + Docker Compose v2 (for local infra and the smoke test via Testcontainers)

## Quick start (dev)

```bash
# 1. Install workspace dependencies
pnpm install

# 2. Configure env
cp .env.example .env            # dev defaults already match the compose file

# 3. Bring up infra (postgres + redis + minio)
pnpm dev:infra                  # docker compose -f docker-compose.dev.yml up -d

# 4. Apply the database schema
pnpm prisma:migrate             # prisma migrate dev (first run creates the DB schema)

# 5. Run the API (hot reload)
pnpm dev:api                    # http mode on :3000

# 6. Verify it's alive (checks Postgres + Redis connectivity)
curl -s http://localhost:3000/health    # → 200 { "status": "ok", ... }
```

Run the **worker** mode instead of the HTTP server:

```bash
API_MODE=worker pnpm --filter @pixela/api run start:dev
```

Stop infra: `pnpm dev:infra:down`.

## Health endpoint

`GET /health` returns **200** only when both Postgres (real `SELECT 1`) and Redis (real
`PING`) are reachable; otherwise **503**. It is the liveness/readiness probe used by
docker-compose and CI. This is the "one green screenshot proves the harness" baseline of
Phase 0.

## Scripts

| Script | What it does |
| --- | --- |
| `pnpm lint` / `pnpm lint:fix` | ESLint across the workspace |
| `pnpm typecheck` | `tsc --noEmit` in every package |
| `pnpm format` / `pnpm format:check` | Prettier |
| `pnpm build` | Build every package |
| `pnpm test` | Run all package tests (API smoke uses Testcontainers — Docker required) |
| `pnpm dev:infra` / `pnpm dev:infra:down` | Start/stop local infra |
| `pnpm prisma:generate` / `pnpm prisma:migrate` | Prisma client / migrations |

## Tests

The Phase 0 smoke test (`apps/api`) spins up **ephemeral** Postgres and Redis containers
via [Testcontainers](https://testcontainers.com/), boots the Nest app, and asserts
`/health` returns 200 with real connectivity. It needs a running Docker daemon but **no**
pre-started infra and leaves no state behind.

```bash
pnpm --filter @pixela/api run test
```
