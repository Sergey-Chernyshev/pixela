# Pixela — Web (`apps/web`)

Angular dashboard for **Pixela**, the self-hosted visual regression testing platform. This is the
frontend workspace of the `pixela` monorepo; the backend is [`apps/api`](../api) and the canonical
spec lives at the repo root under [`docs/spec/`](../../docs/spec/) (see also [`../../CLAUDE.md`](../../CLAUDE.md)
and the frontend-specific [`CLAUDE.md`](CLAUDE.md)).

## Stack

Angular 21 (standalone components, signals, OnPush) · Angular CDK · SCSS · pnpm · Node 22.

## Quick start (from the monorepo root)

```bash
pnpm install                          # installs the whole workspace
pnpm --filter @pixela/web start       # ng serve → http://localhost:4200
pnpm --filter @pixela/web build       # production build into apps/web/dist/
```

The dashboard talks to the Pixela API (`apps/api`, default `http://localhost:3000`). A configurable
API base URL lands in Phase 4.

## Status

**Phase 0** — standalone shell with a lazy-routed Home page; `ng build` is green. The dashboard shell
and review UI (3 comparison modes + synchronized zoom) are **Phase 4** (Agents 06/07); apply the
`frontend-design` skill before writing that UI — see `../../docs/spec/skills/how-to-use-your-skills.md`.
