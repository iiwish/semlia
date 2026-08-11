-- name: CreateWorkspace :one
INSERT INTO workspaces (id, slug, display_name)
VALUES ($1, $2, $3)
RETURNING *;
