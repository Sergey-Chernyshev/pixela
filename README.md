# Pixela (`pixela`)

Self-hosted **visual regression testing** platform — a pnpm/Go monorepo with the **Go** API/worker,
the Angular **dashboard**, and the Playwright **SDK** (TypeScript).

> Full specification: [`docs/spec/`](docs/spec/). Architectural invariants and working agreements:
> [`CLAUDE.md`](CLAUDE.md). Go backend rulebook: [`docs/architecture/go-backend.md`](docs/architecture/go-backend.md)
> (and ADR [`docs/adr/0001-backend-language-go.md`](docs/adr/0001-backend-language-go.md)). Read those
> before changing anything.

## Layout

```
apps/api          Go module — one binary, subcommands: pixela serve | worker | migrate | openapi
apps/web          @pixela/web — Angular 21 standalone dashboard (TypeScript)
packages/sdk      @pixela/playwright-reporter (TypeScript, scaffold)
packages/shared   contract types generated from the API's OpenAPI (scaffold)
docs/spec         the canonical product/engineering specification
docs/architecture/go-backend.md   the binding Go conventions
docker-compose.dev.yml            local infra: postgres + redis (sessions) + minio
```

## Prerequisites

- **Go 1.26** (`apps/api/go.mod` pins the toolchain)
- **Node 22** + **pnpm 10** (`corepack enable`) — for `apps/web` and the TS packages
- **Docker** + Docker Compose v2 — local infra and the Testcontainers integration smoke

## One-command demo

Bring the whole stack up — Postgres, Redis, MinIO, migrations, a seeded demo dataset, the API, the
diff worker, and the dashboard behind nginx — with a single command, then open the browser:

```bash
docker compose up --build           # add -d to run detached
```

- Dashboard: **http://localhost:8080**
- Log in with the credentials printed by the `seed` service:
  **`demo@pixela.dev`** / **`pixela-demo`**

The seed populates one project (`acme-storefront`) with two builds, snapshots across every status
(changed / new / unchanged / removed) with real thumbnails, baselines, members and an activity feed —
so the dashboard is populated out of the box. Host ports are overridable via `.env` (`PIXELA_*_PORT`);
`docker compose down -v` removes the volumes. (For Go API hot-reload during development, use the
infra-only `docker-compose.dev.yml` and the `## Quick start (dev)` flow below instead.)

## Quick start (dev)

```bash
# 1. Bring up infra (postgres + redis + minio)
pnpm dev:infra                     # docker compose -f docker-compose.dev.yml up -d

# 2. Configure env (the Go binary reads the environment, not a .env file)
cp .env.example .env && set -a && . ./.env && set +a   # or use direnv

# 3. Apply the schema + queue tables on a clean DB
pnpm migrate                       # cd apps/api && go run ./cmd/pixela migrate

# 4. Run the API (or `task -d apps/api dev` for hot reload via air)
pnpm dev:api                       # cd apps/api && go run ./cmd/pixela serve  → :3000

# 5. Verify readiness (checks Postgres + Redis + MinIO)
curl -s http://localhost:3000/readyz   # → 200 { "status":"ok","checks":{...} }
```

Run the **worker** (diff-job consumer) instead of the HTTP server:

```bash
cd apps/api && go run ./cmd/pixela worker
```

Run the **dashboard** (`apps/web`): `pnpm dev:web` → http://localhost:4200. Stop infra: `pnpm dev:infra:down`.

## Health probes

- **`GET /healthz`** — liveness: process-only, dependency-free, always 200 (no restart loop on a blip).
- **`GET /readyz`** — readiness: 200 only when Postgres, Redis and MinIO are all reachable, else 503 with
  the failing dependency. Used by docker-compose/CI. This is the "one green screenshot proves the harness"
  baseline of Phase 0.

## Backend tooling (`apps/api`)

[Taskfile](https://taskfile.dev) is the command surface — `task -d apps/api <name>`:

| Task | What it does |
| --- | --- |
| `dev` | hot reload (air), `pixela serve` |
| `build` / `run` | build / run the binary |
| `lint` / `vet` / `fmt` | golangci-lint v2 / `go vet` / gofmt |
| `test` | `go test -race ./...` (unit) |
| `test:integration` | Testcontainers smoke (`-tags=integration`) |
| `generate` | `sqlc generate` + emit `api/openapi.yaml` |
| `migrate` | `pixela migrate` |

Pre-commit gate (also what CI runs): `bash .claude/skills/verify-go/scripts/gate.sh`.

## Tests

The Phase-0 integration smoke (`apps/api/test`, behind `-tags=integration`) spins up **ephemeral**
Postgres + Redis + MinIO via [Testcontainers](https://testcontainers.com/), runs `pixela migrate` on a
clean DB, boots `pixela serve`, and asserts `/readyz` is 200 with every dependency up — then shuts down
cleanly (race-checked). Needs a Docker daemon; leaves no state behind.

```bash
cd apps/api && go test -tags integration -race ./test/...
```

## License

[MIT](LICENSE) © Sergey Chernyshev. Open source — contributions welcome.
