-- name: GetWorkspacePrincipal :one
SELECT * FROM principals
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(principal_id);

-- name: GetPrincipalByID :one
SELECT * FROM principals WHERE id = sqlc.arg(principal_id);

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

-- name: GetLocalUATPrincipal :one
SELECT principal.* FROM workspaces AS workspace
JOIN principals AS principal
  ON principal.workspace_id = workspace.id
 AND principal.id = semlia_seed_uuidv7(
      sqlc.arg(seed_namespace)::text, workspace.id::text, workspace.created_at
 )
JOIN role_bindings AS binding ON binding.principal_id = principal.id
WHERE workspace.id = sqlc.arg(workspace_id)
  AND binding.role_id = sqlc.arg(role_id)
  AND binding.scope_type = 'workspace'
  AND binding.scope_id = workspace.id::text
  AND principal.status = 'active'
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
  AND revoked_at IS NULL
  AND expired_at IS NULL
  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
ORDER BY granted_at, id;

-- name: ListActivePrincipalRoleBindingsAt :many
SELECT * FROM role_bindings
WHERE principal_id = sqlc.arg(principal_id)
  AND revoked_at IS NULL
  AND expired_at IS NULL
  AND (expires_at IS NULL OR expires_at > sqlc.arg(now))
ORDER BY granted_at, id;

-- name: ListActiveRoleBindingsForRoleAt :many
SELECT * FROM role_bindings
WHERE workspace_id = sqlc.arg(workspace_id)
  AND role_id = sqlc.arg(role_id)
  AND role_version = sqlc.arg(role_version)
  AND revoked_at IS NULL
  AND expired_at IS NULL
  AND (expires_at IS NULL OR expires_at > sqlc.arg(now))
ORDER BY principal_id, scope_type, scope_id, id
FOR UPDATE;

-- name: ListRoleActionsForRoles :many
SELECT system_action.role_id, system_action.action FROM role_actions AS system_action
WHERE system_action.role_id = ANY (sqlc.arg(role_ids)::text[])
UNION ALL
SELECT custom.role_id, custom.action
FROM custom_role_version_actions AS custom
JOIN roles AS role
  ON role.workspace_id = custom.workspace_id
 AND role.id = custom.role_id
 AND role.active_version = custom.role_version
WHERE custom.role_id = ANY (sqlc.arg(role_ids)::text[])
ORDER BY role_id, action;

-- name: ListPrincipalRoleBindingActions :many
SELECT binding.id AS binding_id, system_action.action
FROM role_bindings AS binding
JOIN role_actions AS system_action ON system_action.role_id = binding.role_id
WHERE binding.principal_id = sqlc.arg(principal_id)
UNION ALL
SELECT binding.id AS binding_id, custom.action
FROM role_bindings AS binding
JOIN custom_role_version_actions AS custom
  ON custom.workspace_id = binding.workspace_id
 AND custom.role_id = binding.role_id
 AND custom.role_version = binding.role_version
WHERE binding.principal_id = sqlc.arg(principal_id)
ORDER BY binding_id, action;

-- name: ListWorkspaceRoleBindingActions :many
SELECT binding.id AS binding_id, system_action.action
FROM role_bindings AS binding
JOIN role_actions AS system_action ON system_action.role_id = binding.role_id
WHERE binding.workspace_id = sqlc.arg(workspace_id)
UNION ALL
SELECT binding.id AS binding_id, custom.action
FROM role_bindings AS binding
JOIN custom_role_version_actions AS custom
  ON custom.workspace_id = binding.workspace_id
 AND custom.role_id = binding.role_id
 AND custom.role_version = binding.role_version
WHERE binding.workspace_id = sqlc.arg(workspace_id)
ORDER BY binding_id, action;

-- name: CreateRoleBinding :one
INSERT INTO role_bindings (
    id, workspace_id, principal_id, role_id, role_version, scope_type, scope_id,
    granted_by, granted_at, expires_at
)
VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(principal_id), sqlc.arg(role_id),
    sqlc.arg(role_version), sqlc.arg(scope_type), sqlc.arg(scope_id),
    sqlc.narg(granted_by), sqlc.arg(granted_at), sqlc.narg(expires_at)
)
RETURNING *;

-- name: ListAuthorizationRoles :many
SELECT
    role.id, role.workspace_id, role.name, role.description, role.category,
    COALESCE(role.active_version, 1)::bigint AS version,
    version.created_at
FROM roles AS role
LEFT JOIN custom_role_versions AS version
  ON version.workspace_id = role.workspace_id
 AND version.role_id = role.id
 AND version.version = role.active_version
WHERE role.category = 'system' OR role.workspace_id = sqlc.arg(workspace_id)
ORDER BY role.category DESC, role.name, role.id;

-- name: GetAuthorizationRole :one
SELECT
    role.id, role.workspace_id, role.name, role.description, role.category,
    COALESCE(role.active_version, 1)::bigint AS version,
    version.created_at
FROM roles AS role
LEFT JOIN custom_role_versions AS version
  ON version.workspace_id = role.workspace_id
 AND version.role_id = role.id
 AND version.version = role.active_version
WHERE role.id = sqlc.arg(role_id)
  AND (role.category = 'system' OR role.workspace_id = sqlc.arg(workspace_id));

-- name: CreateCustomRoleBase :exec
INSERT INTO roles (id, workspace_id, name, description, category, active_version)
VALUES (
    sqlc.arg(role_id), sqlc.arg(workspace_id), sqlc.arg(name),
    sqlc.arg(description), 'custom', 1
);

-- name: CreateCustomRoleVersion :exec
INSERT INTO custom_role_versions (
    id, workspace_id, role_id, version, name, description, created_by, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(role_id), sqlc.arg(version),
    sqlc.arg(name), sqlc.arg(description), sqlc.arg(created_by), sqlc.arg(created_at)
);

-- name: CreateCustomRoleVersionAction :exec
INSERT INTO custom_role_version_actions (workspace_id, role_id, role_version, action)
VALUES (sqlc.arg(workspace_id), sqlc.arg(role_id), sqlc.arg(role_version), sqlc.arg(action));

-- name: AdvanceCustomRoleVersion :one
UPDATE roles
SET name = sqlc.arg(name), description = sqlc.arg(description),
    active_version = sqlc.arg(next_version)
WHERE workspace_id = sqlc.arg(workspace_id)
  AND id = sqlc.arg(role_id)
  AND category = 'custom'
  AND active_version = sqlc.arg(expected_version)
RETURNING active_version;

-- name: AdvanceActiveCustomRoleBindings :execrows
UPDATE role_bindings
SET role_version = sqlc.arg(next_version), version = version + 1
WHERE workspace_id = sqlc.arg(workspace_id)
  AND role_id = sqlc.arg(role_id)
  AND role_version = sqlc.arg(expected_version)
  AND revoked_at IS NULL
  AND expired_at IS NULL
  AND (expires_at IS NULL OR expires_at > sqlc.arg(now));

-- name: ListAuthorizationRoleBindings :many
SELECT
    binding.*, principal.display_name AS principal_display_name,
    role.category AS role_category,
    COALESCE(role.active_version, 1)::bigint AS current_role_version
FROM role_bindings AS binding
JOIN principals AS principal
  ON principal.workspace_id = binding.workspace_id AND principal.id = binding.principal_id
JOIN roles AS role ON role.id = binding.role_id
WHERE binding.workspace_id = sqlc.arg(workspace_id)
ORDER BY binding.granted_at DESC, binding.id DESC;

-- name: GetAuthorizationRoleBinding :one
SELECT
    binding.*, principal.display_name AS principal_display_name,
    role.category AS role_category,
    COALESCE(role.active_version, 1)::bigint AS current_role_version
FROM role_bindings AS binding
JOIN principals AS principal
  ON principal.workspace_id = binding.workspace_id AND principal.id = binding.principal_id
JOIN roles AS role ON role.id = binding.role_id
WHERE binding.workspace_id = sqlc.arg(workspace_id) AND binding.id = sqlc.arg(binding_id);

-- name: RevokeAuthorizationRoleBinding :one
UPDATE role_bindings
SET revoked_at = sqlc.arg(revoked_at), revoked_by = sqlc.arg(revoked_by),
    revocation_reason = sqlc.arg(revocation_reason), version = version + 1
WHERE workspace_id = sqlc.arg(workspace_id)
  AND id = sqlc.arg(binding_id)
  AND version = sqlc.arg(expected_version)
  AND revoked_at IS NULL
RETURNING *;

-- name: ExpireAuthorizationRoleBindings :many
UPDATE role_bindings
SET expired_at = expires_at, version = version + 1
WHERE workspace_id = sqlc.arg(workspace_id)
  AND revoked_at IS NULL
  AND expired_at IS NULL
  AND expires_at IS NOT NULL
  AND expires_at <= sqlc.arg(expired_at)
RETURNING id;

-- name: CountActiveAuthorizationAdministrators :one
SELECT count(*)
FROM role_bindings AS binding
JOIN principals AS principal
  ON principal.workspace_id = binding.workspace_id AND principal.id = binding.principal_id
JOIN workspace_memberships AS membership
  ON membership.workspace_id = binding.workspace_id
 AND membership.principal_id = binding.principal_id
WHERE binding.workspace_id = sqlc.arg(workspace_id)
  AND binding.role_id = 'workspace_admin'
  AND binding.scope_type = 'workspace'
  AND binding.scope_id = binding.workspace_id::text
  AND binding.revoked_at IS NULL
  AND binding.expired_at IS NULL
  AND (binding.expires_at IS NULL OR binding.expires_at > sqlc.arg(now))
  AND principal.kind = 'human'
  AND principal.status = 'active'
  AND membership.status = 'active';

-- name: IsActiveWorkspaceAdministrator :one
SELECT EXISTS (
    SELECT 1
    FROM role_bindings AS binding
    JOIN principals AS principal
      ON principal.workspace_id = binding.workspace_id AND principal.id = binding.principal_id
    JOIN workspace_memberships AS membership
      ON membership.workspace_id = binding.workspace_id
     AND membership.principal_id = binding.principal_id
    WHERE binding.workspace_id = sqlc.arg(workspace_id)
      AND binding.principal_id = sqlc.arg(principal_id)
      AND binding.role_id = 'workspace_admin'
      AND binding.scope_type = 'workspace'
      AND binding.scope_id = binding.workspace_id::text
      AND binding.revoked_at IS NULL
      AND binding.expired_at IS NULL
      AND (binding.expires_at IS NULL OR binding.expires_at > sqlc.arg(now))
      AND principal.kind = 'human'
      AND principal.status = 'active'
      AND membership.status = 'active'
);

-- name: LockAuthorizationWorkspace :one
SELECT authorization_version FROM workspaces
WHERE id = sqlc.arg(workspace_id)
FOR UPDATE;

-- name: AuthorizationScopeExists :one
SELECT CASE sqlc.arg(scope_type)::text
    WHEN 'workspace' THEN EXISTS (
        SELECT 1 FROM workspaces AS workspace
        WHERE workspace.id::text = sqlc.arg(scope_id)::text AND workspace.id = sqlc.arg(workspace_id)
    )
    WHEN 'asset' THEN EXISTS (
        SELECT 1 FROM semantic_assets AS asset
        WHERE asset.id::text = sqlc.arg(scope_id)::text AND asset.workspace_id = sqlc.arg(workspace_id)
    )
    WHEN 'source' THEN EXISTS (
        SELECT 1 FROM source_connections AS source
        WHERE source.id::text = sqlc.arg(scope_id)::text AND source.workspace_id = sqlc.arg(workspace_id)
    )
    WHEN 'release' THEN EXISTS (
        SELECT 1 FROM releases AS release
        WHERE release.id::text = sqlc.arg(scope_id)::text AND release.workspace_id = sqlc.arg(workspace_id)
    )
    WHEN 'consumer' THEN EXISTS (
        SELECT 1 FROM consumers AS consumer
        WHERE consumer.id::text = sqlc.arg(scope_id)::text AND consumer.workspace_id = sqlc.arg(workspace_id)
    )
    WHEN 'domain' THEN true
    WHEN 'environment' THEN true
    ELSE false
END;

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
