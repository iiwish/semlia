-- M2-T008 governance objects. All four tables share one shape: a workspace
-- scope, a TypeID identity, a version bumped only by the governed change
-- application path, a flexible content object and the shared
-- create/get/list/lock/apply pipeline.

-- name: CreatePhysicalBinding :one
INSERT INTO physical_bindings (
    id, workspace_id, asset_id, dataset_id, field_id, transform, retired_at,
    version, content, created_by, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(asset_id), sqlc.arg(dataset_id),
    sqlc.narg(field_id), sqlc.narg(transform), sqlc.narg(retired_at), sqlc.arg(version),
    sqlc.arg(content), sqlc.arg(created_by), sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: GetPhysicalBinding :one
SELECT * FROM physical_bindings
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(physical_binding_id);

-- name: ListPhysicalBindings :many
SELECT * FROM physical_bindings
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY updated_at DESC, id;

-- name: LockPhysicalBindingForUpdate :one
SELECT version FROM physical_bindings
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(physical_binding_id)
FOR UPDATE;

-- name: ApplyPhysicalBindingChange :execrows
UPDATE physical_bindings
SET content = sqlc.arg(content), version = version + 1, updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(physical_binding_id)
  AND version = sqlc.arg(expected_version);

-- name: PhysicalBindingExists :one
SELECT EXISTS (
    SELECT 1 FROM physical_bindings
    WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(physical_binding_id)
) AS present;

-- name: CreateModelGrain :one
INSERT INTO model_grains (
    id, workspace_id, asset_id, grain_expression, grain_field_refs, documented_by,
    version, content, created_by, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(asset_id), sqlc.arg(grain_expression),
    sqlc.arg(grain_field_refs), sqlc.narg(documented_by), sqlc.arg(version), sqlc.arg(content),
    sqlc.arg(created_by), sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: GetModelGrain :one
SELECT * FROM model_grains
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(model_grain_id);

-- name: ListModelGrains :many
SELECT * FROM model_grains
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY updated_at DESC, id;

-- name: LockModelGrainForUpdate :one
SELECT version FROM model_grains
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(model_grain_id)
FOR UPDATE;

-- name: ApplyModelGrainChange :execrows
UPDATE model_grains
SET content = sqlc.arg(content), version = version + 1, updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(model_grain_id)
  AND version = sqlc.arg(expected_version);

-- name: ModelGrainExists :one
SELECT EXISTS (
    SELECT 1 FROM model_grains
    WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(model_grain_id)
) AS present;

-- name: CreateEntityKey :one
INSERT INTO entity_keys (
    id, workspace_id, asset_id, key_field_refs, uniqueness_semantics,
    version, content, created_by, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(asset_id), sqlc.arg(key_field_refs),
    sqlc.arg(uniqueness_semantics), sqlc.arg(version), sqlc.arg(content),
    sqlc.arg(created_by), sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: GetEntityKey :one
SELECT * FROM entity_keys
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(entity_key_id);

-- name: ListEntityKeys :many
SELECT * FROM entity_keys
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY updated_at DESC, id;

-- name: LockEntityKeyForUpdate :one
SELECT version FROM entity_keys
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(entity_key_id)
FOR UPDATE;

-- name: ApplyEntityKeyChange :execrows
UPDATE entity_keys
SET content = sqlc.arg(content), version = version + 1, updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(entity_key_id)
  AND version = sqlc.arg(expected_version);

-- name: EntityKeyExists :one
SELECT EXISTS (
    SELECT 1 FROM entity_keys
    WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(entity_key_id)
) AS present;

-- name: CreateJoinContract :one
INSERT INTO join_contracts (
    id, workspace_id, left_dataset_id, right_dataset_id, left_field_refs, right_field_refs,
    join_type, cardinality, join_expression, contract_notes,
    version, content, created_by, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(left_dataset_id), sqlc.arg(right_dataset_id),
    sqlc.arg(left_field_refs), sqlc.arg(right_field_refs), sqlc.arg(join_type),
    sqlc.arg(cardinality), sqlc.arg(join_expression), sqlc.narg(contract_notes),
    sqlc.arg(version), sqlc.arg(content), sqlc.arg(created_by), sqlc.arg(created_at),
    sqlc.arg(updated_at)
)
RETURNING *;

-- name: GetJoinContract :one
SELECT * FROM join_contracts
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(join_contract_id);

-- name: ListJoinContracts :many
SELECT * FROM join_contracts
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY updated_at DESC, id;

-- name: LockJoinContractForUpdate :one
SELECT version FROM join_contracts
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(join_contract_id)
FOR UPDATE;

-- name: ApplyJoinContractChange :execrows
UPDATE join_contracts
SET content = sqlc.arg(content), version = version + 1, updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(join_contract_id)
  AND version = sqlc.arg(expected_version);

-- name: JoinContractExists :one
SELECT EXISTS (
    SELECT 1 FROM join_contracts
    WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(join_contract_id)
) AS present;
