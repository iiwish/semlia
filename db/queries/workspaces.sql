-- name: CreateWorkspace :one
INSERT INTO workspaces (id, slug, display_name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListWorkspaces :many
SELECT workspace.*
FROM workspaces AS workspace
ORDER BY EXISTS (
    SELECT 1
    FROM semantic_assets AS asset
    WHERE asset.workspace_id = workspace.id
) DESC, workspace.created_at DESC, workspace.id DESC;
