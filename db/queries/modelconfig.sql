-- M2-T009 model configuration queries. Credential material never appears:
-- only the env-var NAME and the revision digest are stored or read.

-- name: CreateModelProvider :one
INSERT INTO model_providers (
    id, workspace_id, protocol, display_name, base_url,
    credential_env, credential_revision, enabled, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(protocol), sqlc.arg(display_name),
    sqlc.narg(base_url), sqlc.arg(credential_env), sqlc.arg(credential_revision),
    sqlc.arg(enabled), sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: GetModelProvider :one
SELECT * FROM model_providers
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(provider_id);

-- name: ListModelProviders :many
SELECT * FROM model_providers
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY created_at, id;

-- name: UpdateModelProvider :one
UPDATE model_providers
SET display_name = sqlc.arg(display_name), base_url = sqlc.narg(base_url),
    credential_env = sqlc.arg(credential_env),
    credential_revision = sqlc.arg(credential_revision),
    enabled = sqlc.arg(enabled), updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(provider_id)
RETURNING *;

-- name: CreateModelSetting :one
INSERT INTO model_settings (
    id, workspace_id, provider_id, kind, model, enabled, is_default,
    capability, token_limit, embedding_dimension, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(provider_id), sqlc.arg(kind),
    sqlc.arg(model), sqlc.arg(enabled), sqlc.arg(is_default), sqlc.arg(capability),
    sqlc.arg(token_limit), sqlc.narg(embedding_dimension), sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: GetModelSetting :one
SELECT * FROM model_settings
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(setting_id);

-- name: ListModelSettings :many
SELECT * FROM model_settings
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY provider_id, created_at, id;

-- name: UpdateModelSetting :one
UPDATE model_settings
SET model = sqlc.arg(model), enabled = sqlc.arg(enabled),
    capability = sqlc.arg(capability), token_limit = sqlc.arg(token_limit),
    embedding_dimension = sqlc.narg(embedding_dimension), updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(setting_id)
RETURNING *;

-- name: ClearModelSettingDefaults :exec
UPDATE model_settings
SET is_default = false, updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND kind = sqlc.arg(kind)
  AND id <> sqlc.arg(setting_id);

-- name: SetModelSettingDefault :one
UPDATE model_settings
SET is_default = true, updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(setting_id)
RETURNING *;

-- name: GetDefaultModelSetting :one
SELECT setting.* FROM model_settings AS setting
JOIN model_providers AS provider
  ON provider.workspace_id = setting.workspace_id AND provider.id = setting.provider_id
WHERE setting.workspace_id = sqlc.arg(workspace_id) AND setting.kind = sqlc.arg(kind)
  AND setting.is_default AND setting.enabled AND provider.enabled
LIMIT 1;

-- name: GetWorkspaceAgentPrincipal :one
SELECT principal.* FROM principals AS principal
WHERE principal.workspace_id = sqlc.arg(workspace_id) AND principal.kind = 'agent'
ORDER BY principal.created_at, principal.id
LIMIT 1;
