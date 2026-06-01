# ADR 0002 — Baseline ownership: git-native (Mode A), not server-owned (Mode B)

- **Status:** Accepted (2026-06-02)
- **Deciders:** Sergey Chernyshev (owner)
- **Reaffirms:** product invariant #1 in `CLAUDE.md` ("git-native baseline; NO server merge-base resolution").
- **Does not change:** the data model, the API contract, the reporter, or the dashboard. It records *why* the baseline lives in the test repo and what "approve" does.

## Context

The owner asked the natural question: wouldn't it be cleaner to store baselines **only** on the Pixela server and pull them at test time, keeping the test repo lean? That is the choice between two well-known models:

- **Mode A (git-native, current):** baseline PNGs are committed to the test repo exactly where Playwright stores them. `toHaveScreenshot` compares locally against the committed file; Pixela is a review layer. "Approve" prepares a git commit of the updated baseline back into the branch.
- **Mode B (server-owned):** baselines live only in Pixela; at test time the correct baseline is resolved and fetched from the server (or Pixela compares server-side). The repo holds no baselines.

Note: **Pixela already stores the full image history server-side** (new + baseline + diff in MinIO, metadata in Postgres) under *both* models. The decision is narrow — it is only about where the **comparison baseline** (the pass/fail gate) lives, not about where history lives.

## Decision

**Keep Mode A: the test repository owns the baseline.** The recommended setup is **Mode A + Git LFS** for the snapshot directory, which keeps the working tree lean (LFS pointers in the pack, PNGs in the LFS store) while preserving the properties below. Pixela still holds the full history centrally regardless.

"Approve" (Phase 5) means: Pixela writes the approved new baseline PNG **into the feature branch** (a commit pushed to the same branch — ideal, so the PR carries the code change *and* its baseline atomically — or a small MR targeting that branch when direct push is undesirable). This requires Pixela to hold a **git write credential** (deploy key / token) for the repo. After that commit the visual job goes green and the PR is mergeable.

## Consequences

**Positive (why Mode A):**
- **Determinism / hermetic runs:** the baseline is whatever is in the tree at that commit — no runtime resolution, no network dependency on Pixela during CI. If Pixela is down, tests still run. (Invariant #6: determinism > features.)
- **No "which baseline?" problem:** Mode B must resolve the baseline at runtime by branch / merge-base — exactly the server merge-base resolution invariant #1 forbids, and the classic source of "the tool sometimes lies."
- **Atomicity:** a PR that changes the UI carries the matching baseline change in the same commit, reviewable in one diff. Code and its expectation never drift apart.
- **Free version control:** git already gives branching, merge, history and rollback of baselines (revert the commit → baseline reverts). Mode B reimplements that in a mutable server DB.

**Negative / accepted costs:**
- The repo stores binary PNGs → **Git LFS** is effectively required to avoid pack bloat.
- "Approve" is a git round-trip (commit + re-run pipeline), slower than a Mode-B instant DB flip.
- Pixela needs **git push access** to the repo to write baselines — an integration prerequisite and a credential to secure.

**Revisit if:** repo cleanliness + instant approve genuinely outweigh determinism and atomicity for the owner — then move to Mode B *consciously*, accepting that it requires a robust baseline-resolution layer (parent-commit / branch baselines, auto-approve-on-default-branch) and that CI runs become coupled to Pixela's availability. This is the path Percy / Chromatic / Argos took; it is a real product, not a footgun, but it is a different product with a much larger baseline-resolution surface.

## Status of implementation

Mode A's *ingestion + review* half is built (reporter, ingestion, async diff, dashboard). The *approve* half — writing the baseline back to git, the approve/reject endpoints, and GitLab MR status — is **Phase 5, not yet implemented**. Until then the regression loop is open (no server-side baseline registration, approve buttons are stubs). See `docs/integration.md` §11.
