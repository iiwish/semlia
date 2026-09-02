-- name: GetWorkspacePrincipal :one
SELECT * FROM principals
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(principal_id);

-- name: GetDefaultWorkspacePrincipal :one
SELECT principal.* FROM principals AS principal
JOIN role_bindings AS binding ON binding.principal_id = principal.id
WHERE principal.workspace_id = sqlc.arg(workspace_id)
  AND binding.role_id = 'workspace_admin'
  AND binding.scope_type = 'workspace'
  AND binding.scope_id = sqlc.arg(workspace_id)::text
  AND binding.granted_by IS NULL
ORDER BY principal.created_at, principal.id
LIMIT 1;

-- name: CreatePrincipal :one
INSERT INTO principals (id, workspace_id, kind, display_name, owner_principal_id, status)
VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(kind), sqlc.arg(display_name),
    sqlc.narg(owner_principal_id), sqlc.arg(status)
)
RETURNING *;

-- name: ListPrincipalRoleBindings :many
SELECT * FROM role_bindings
WHERE principal_id = sqlc.arg(principal_id)
ORDER BY granted_at, id;

-- name: ListRoleActionsForRoles :many
SELECT role_id, action FROM role_actions
WHERE role_id = ANY (sqlc.arg(role_ids)::text[]);

-- name: CreateRoleBinding :one
INSERT INTO role_bindings (id, principal_id, role_id, scope_type, scope_id, granted_by)
VALUES (
    sqlc.arg(id), sqlc.arg(principal_id), sqlc.arg(role_id), sqlc.arg(scope_type),
    sqlc.arg(scope_id), sqlc.narg(granted_by)
)
RETURNING *;

-- name: GetWorkspaceAuthorizationVersion :one
SELECT authorization_version FROM workspaces WHERE id = sqlc.arg(workspace_id);

-- name: BumpWorkspaceAuthorizationVersion :one
UPDATE workspaces
SET authorization_version = authorization_version + 1, updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(workspace_id)
RETURNING authorization_version;

-- name: CreateAuthorizationEvent :exec
INSERT INTO authorization_events (
    id, workspace_id, principal_id, actor, action, resource_type, resource_id,
    decision, reason_code, authorization_version, trace_id, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.narg(principal_id), sqlc.arg(actor),
    sqlc.arg(action), sqlc.arg(resource_type), sqlc.arg(resource_id), sqlc.arg(decision),
    sqlc.arg(reason_code), sqlc.arg(authorization_version), sqlc.arg(trace_id), sqlc.arg(created_at)
);
