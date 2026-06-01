-- Pixela schema — single source of truth for sqlc (schema:) and Atlas (src:).
-- 1:1 with docs/spec/specs/03-data-model.md. Migrations are irreversible in prod — change with care.
-- IDs are application-generated cuid2 TEXT (Prisma @default(cuid())); no uuid/serial.

CREATE TYPE build_status AS ENUM (
  'RUNNING', 'COMPARING', 'PASSED', 'REVIEW_REQUIRED', 'REJECTED', 'ERROR'
);

CREATE TYPE snapshot_status AS ENUM (
  'PENDING', 'UNCHANGED', 'CHANGED', 'NEW', 'REMOVED', 'APPROVED', 'REJECTED', 'ERROR'
);

CREATE TYPE approval_action AS ENUM ('APPROVE', 'REJECT');

CREATE TYPE role AS ENUM ('OWNER', 'MEMBER');

-- Content-addressable image metadata. Bytes live in S3/MinIO under key = sha256.
CREATE TABLE images (
  sha256     TEXT        PRIMARY KEY,
  width      INTEGER     NOT NULL,
  height     INTEGER     NOT NULL,
  byte_size  INTEGER     NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
  id                TEXT        PRIMARY KEY,
  name              TEXT        NOT NULL,
  slug              TEXT        NOT NULL UNIQUE,
  default_branch    TEXT        NOT NULL DEFAULT 'main',
  -- GitLab project id or path (e.g. "acme/storefront"); when set, approve commits the new baseline
  -- back to this repo and build status is mirrored to the MR (Phase 5, Mode A / git-native).
  gitlab_project_id TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
  id            TEXT        PRIMARY KEY,
  email         TEXT        NOT NULL UNIQUE,
  name          TEXT,
  password_hash TEXT,
  gitlab_id     TEXT        UNIQUE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Store the HASH of the API key, never the key itself.
CREATE TABLE api_keys (
  id           TEXT        PRIMARY KEY,
  project_id   TEXT        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  key_hash     TEXT        NOT NULL UNIQUE,
  name         TEXT        NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ
);

-- One active baseline per (project, branch, name, browser, viewport).
CREATE TABLE baselines (
  id                  TEXT        PRIMARY KEY,
  project_id          TEXT        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  branch              TEXT        NOT NULL,
  name                TEXT        NOT NULL,
  browser             TEXT        NOT NULL,
  viewport            TEXT        NOT NULL,
  image_sha           TEXT        NOT NULL REFERENCES images (sha256),
  approved_by_user_id TEXT,
  approved_in_build_id TEXT,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT baselines_project_branch_name_browser_viewport_key
    UNIQUE (project_id, branch, name, browser, viewport)
);

CREATE TABLE builds (
  id           TEXT         PRIMARY KEY,
  project_id   TEXT         NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  branch       TEXT         NOT NULL,
  commit_sha   TEXT         NOT NULL,
  ci_build_id  TEXT,
  ci_job_url   TEXT,
  mr_iid       TEXT,
  status       build_status NOT NULL DEFAULT 'RUNNING',
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
  finalized_at TIMESTAMPTZ
);

CREATE INDEX builds_project_branch_created_idx ON builds (project_id, branch, created_at);
CREATE INDEX builds_project_commit_idx ON builds (project_id, commit_sha);

CREATE TABLE snapshots (
  id             TEXT            PRIMARY KEY,
  build_id       TEXT            NOT NULL REFERENCES builds (id) ON DELETE CASCADE,
  name           TEXT            NOT NULL,
  browser        TEXT            NOT NULL,
  viewport       TEXT            NOT NULL,
  new_image_sha  TEXT            REFERENCES images (sha256) ON DELETE SET NULL,
  diff_image_sha TEXT            REFERENCES images (sha256) ON DELETE SET NULL,
  baseline_id    TEXT            REFERENCES baselines (id) ON DELETE SET NULL,
  diff_ratio     DOUBLE PRECISION,
  diff_pixels    INTEGER,
  status         snapshot_status NOT NULL DEFAULT 'PENDING',
  error_msg      TEXT,
  -- Repo-relative path of this snapshot's baseline file (Playwright snapshot path), reported by the
  -- reporter. On approve, Pixela commits the new image to this path on the build's branch (Mode A).
  baseline_path  TEXT,
  created_at     TIMESTAMPTZ     NOT NULL DEFAULT now(),
  CONSTRAINT snapshots_build_name_browser_viewport_key
    UNIQUE (build_id, name, browser, viewport)
);

CREATE INDEX snapshots_status_idx ON snapshots (status);

CREATE TABLE approval_events (
  id          TEXT            PRIMARY KEY,
  snapshot_id TEXT            NOT NULL REFERENCES snapshots (id) ON DELETE CASCADE,
  user_id     TEXT            NOT NULL REFERENCES users (id),
  action      approval_action NOT NULL,
  created_at  TIMESTAMPTZ     NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
  id         TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
  role       role NOT NULL DEFAULT 'MEMBER',
  CONSTRAINT memberships_user_project_key UNIQUE (user_id, project_id)
);
