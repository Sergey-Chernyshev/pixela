-- Projects & API keys (bootstrap/admin surface; dashboard endpoints get session auth in Phase 4).

-- name: CreateProject :one
INSERT INTO projects (id, name, slug, default_branch)
VALUES (@id, @name, @slug, @default_branch)
RETURNING *;

-- name: GetProjectBySlug :one
SELECT * FROM projects WHERE slug = @slug;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = @id;

-- name: CreateAPIKey :one
INSERT INTO api_keys (id, project_id, key_hash, name)
VALUES (@id, @project_id, @key_hash, @name)
RETURNING *;

-- name: GetAPIKeyByHash :one
SELECT id, project_id FROM api_keys WHERE key_hash = @key_hash;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = now() WHERE id = @id;
