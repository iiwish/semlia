-- name: InsertProductionOperation :one
INSERT INTO production_operations (
    id, workspace_id, created_by, current_version, created_at, updated_at, supersedes_operation_id
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(created_by), sqlc.arg(current_version),
    sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.narg(supersedes_operation_id)
)
RETURNING *;

-- name: GetProductionOperation :one
SELECT * FROM production_operations
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(id);

-- name: GetProductionOperationForUpdate :one
SELECT * FROM production_operations
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(id)
FOR UPDATE;

-- name: UpdateProductionOperationVersion :exec
UPDATE production_operations
SET current_version = sqlc.arg(current_version),
    updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(id);

-- name: ListProductionOperations :many
SELECT * FROM production_operations
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: InsertProductionVersion :one
INSERT INTO production_versions (
    workspace_id, operation_id, version, input_json, input_digest,
    request_digest, set_digest, frozen_at, created_by, created_at,
    history_quality, canonical_input, canonical_declarations, canonical_baseline, baseline_head, baseline_digest
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(operation_id), sqlc.arg(version),
    sqlc.arg(input_json), sqlc.arg(input_digest), sqlc.arg(request_digest),
    sqlc.arg(set_digest), sqlc.narg(frozen_at), sqlc.arg(created_by), sqlc.arg(created_at),
    'verified', sqlc.arg(canonical_input), sqlc.arg(canonical_declarations), sqlc.arg(canonical_baseline), sqlc.arg(baseline_head), sqlc.arg(baseline_digest)
)
RETURNING *;

-- name: GetProductionVersion :one
SELECT * FROM production_versions
WHERE workspace_id = sqlc.arg(workspace_id)
  AND operation_id = sqlc.arg(operation_id)
  AND version = sqlc.arg(version);

-- name: InsertProductionTarget :exec
INSERT INTO production_targets (
    workspace_id, operation_id, version, local_key, kind, intent,
    target_id, identity_key, base_revision_id, base_object_version,
    registry_write_version, content_json, content_digest, proposal_id, outcome, canonical_content
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(operation_id), sqlc.arg(version),
    sqlc.arg(local_key), sqlc.arg(kind), sqlc.arg(intent),
    sqlc.arg(target_id), sqlc.narg(identity_key), sqlc.narg(base_revision_id),
    sqlc.narg(base_object_version), sqlc.narg(registry_write_version),
    sqlc.arg(content_json), sqlc.arg(content_digest), sqlc.narg(proposal_id),
    sqlc.arg(outcome), sqlc.arg(canonical_content)
);

-- name: ListProductionTargetsForVersion :many
SELECT * FROM production_targets
WHERE workspace_id = sqlc.arg(workspace_id)
  AND operation_id = sqlc.arg(operation_id)
  AND version = sqlc.arg(version)
ORDER BY local_key ASC;

-- name: InsertProductionCandidateLink :exec
INSERT INTO production_candidate_links (
    workspace_id, candidate_id, candidate_digest, operation_id, version, local_key, decision_id, is_primary
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(candidate_id), sqlc.arg(candidate_digest),
    sqlc.arg(operation_id), sqlc.arg(version), sqlc.arg(local_key),
    sqlc.narg(decision_id), sqlc.arg(is_primary)
);

-- name: ListProductionCandidateLinksForVersion :many
SELECT * FROM production_candidate_links
WHERE workspace_id = sqlc.arg(workspace_id)
  AND operation_id = sqlc.arg(operation_id)
  AND version = sqlc.arg(version)
ORDER BY local_key ASC, candidate_id ASC;

-- name: InsertProductionContributor :exec
INSERT INTO production_contributors (
    workspace_id, operation_id, principal_id, role, created_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(operation_id), sqlc.arg(principal_id),
    sqlc.arg(role), sqlc.arg(created_at)
)
ON CONFLICT (workspace_id, operation_id, principal_id) DO NOTHING;

-- name: ListProductionContributors :many
SELECT * FROM production_contributors
WHERE workspace_id = sqlc.arg(workspace_id) AND operation_id = sqlc.arg(operation_id)
ORDER BY created_at ASC;

-- name: InsertProductionReservation :exec
INSERT INTO production_identity_reservations (
    workspace_id, kind, identity_key, target_id, creation_operation_id, owner_operation_id, created_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(kind), sqlc.arg(identity_key),
    sqlc.arg(target_id), sqlc.arg(creation_operation_id), sqlc.arg(owner_operation_id),
    sqlc.arg(created_at)
) ON CONFLICT (workspace_id, kind, identity_key) DO NOTHING;

-- name: GetProductionReservation :one
SELECT * FROM production_identity_reservations
WHERE workspace_id = sqlc.arg(workspace_id)
  AND kind = sqlc.arg(kind)
  AND identity_key = sqlc.arg(identity_key);

-- name: UpdateProductionReservationOwner :exec
UPDATE production_identity_reservations
SET owner_operation_id = sqlc.arg(owner_operation_id)
WHERE workspace_id = sqlc.arg(workspace_id)
  AND kind = sqlc.arg(kind)
  AND identity_key = sqlc.arg(identity_key)
  AND owner_operation_id = sqlc.arg(expected_owner_operation_id);

-- name: InsertProductionCommand :exec
INSERT INTO production_commands (
    workspace_id, principal_id, command_kind, idempotency_key, request_digest,
    operation_id, operation_version, result_kind, result_id, committed_at, response_json
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(principal_id), sqlc.arg(command_kind),
    sqlc.arg(idempotency_key), sqlc.arg(request_digest), sqlc.arg(operation_id),
    sqlc.arg(operation_version), sqlc.arg(result_kind), sqlc.arg(result_id),
    sqlc.arg(committed_at), sqlc.narg(response_json)
);

-- name: GetProductionCommand :one
SELECT * FROM production_commands
WHERE workspace_id = sqlc.arg(workspace_id)
  AND principal_id = sqlc.arg(principal_id)
  AND command_kind = sqlc.arg(command_kind)
  AND idempotency_key = sqlc.arg(idempotency_key);

-- name: InsertProductionRequestClaim :exec
INSERT INTO production_request_claims (
    workspace_id, business_digest, operation_id, created_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(business_digest), sqlc.arg(operation_id), sqlc.arg(created_at)
);

-- name: GetProductionRequestClaim :one
SELECT * FROM production_request_claims
WHERE workspace_id = sqlc.arg(workspace_id)
  AND business_digest = sqlc.arg(business_digest);

-- name: FreezeProductionVersion :exec
UPDATE production_versions
SET frozen_at = sqlc.arg(frozen_at)
WHERE workspace_id = sqlc.arg(workspace_id)
  AND operation_id = sqlc.arg(operation_id)
  AND version = sqlc.arg(version)
  AND frozen_at IS NULL;

-- name: SetActiveValidationAttempt :exec
UPDATE production_versions
SET active_validation_attempt_no = sqlc.arg(attempt_no)
WHERE workspace_id = sqlc.arg(workspace_id)
  AND operation_id = sqlc.arg(operation_id)
  AND version = sqlc.arg(version);

-- name: InsertProductionValidationAttempt :one
INSERT INTO production_validation_attempts (
    workspace_id, operation_id, production_version, attempt_no,
    set_digest, input_digest, freshness_witness_json, freshness_digest,
    required_checks_json, required_checks_digest, status, validation_digest,
    created_by, created_at, completed_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(operation_id), sqlc.arg(production_version), sqlc.arg(attempt_no),
    sqlc.arg(set_digest), sqlc.arg(input_digest), sqlc.arg(freshness_witness_json), sqlc.arg(freshness_digest),
    sqlc.arg(required_checks_json), sqlc.arg(required_checks_digest), sqlc.arg(status), sqlc.narg(validation_digest),
    sqlc.arg(created_by), sqlc.arg(created_at), sqlc.narg(completed_at)
)
RETURNING *;

-- name: GetProductionValidationAttempt :one
SELECT * FROM production_validation_attempts
WHERE workspace_id = sqlc.arg(workspace_id)
  AND operation_id = sqlc.arg(operation_id)
  AND production_version = sqlc.arg(production_version)
  AND attempt_no = sqlc.arg(attempt_no);

-- name: GetLatestProductionValidationAttempt :one
SELECT * FROM production_validation_attempts
WHERE workspace_id = sqlc.arg(workspace_id)
  AND operation_id = sqlc.arg(operation_id)
  AND production_version = sqlc.arg(production_version)
ORDER BY attempt_no DESC
LIMIT 1;

-- name: ListProductionValidationAttempts :many
SELECT * FROM production_validation_attempts
WHERE workspace_id = sqlc.arg(workspace_id)
  AND operation_id = sqlc.arg(operation_id)
  AND production_version = sqlc.arg(production_version)
ORDER BY attempt_no DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpdateProductionValidationAttemptStatus :exec
UPDATE production_validation_attempts
SET status = sqlc.arg(status),
    validation_digest = sqlc.narg(validation_digest),
    completed_at = sqlc.narg(completed_at)
WHERE workspace_id = sqlc.arg(workspace_id)
  AND operation_id = sqlc.arg(operation_id)
  AND production_version = sqlc.arg(production_version)
  AND attempt_no = sqlc.arg(attempt_no);

-- name: InsertProductionValidationBinding :exec
INSERT INTO production_validation_bindings (
    workspace_id, validation_run_id, operation_id, production_version, attempt_no,
    set_digest, proposal_id, proposal_content_digest, input_digest, created_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(validation_run_id), sqlc.arg(operation_id), sqlc.arg(production_version),
    sqlc.arg(attempt_no), sqlc.arg(set_digest), sqlc.arg(proposal_id),
    sqlc.arg(proposal_content_digest), sqlc.arg(input_digest), sqlc.arg(created_at)
);

-- name: ListProductionValidationBindingsForAttempt :many
SELECT * FROM production_validation_bindings
WHERE workspace_id = sqlc.arg(workspace_id)
  AND operation_id = sqlc.arg(operation_id)
  AND production_version = sqlc.arg(production_version)
  AND attempt_no = sqlc.arg(attempt_no);

-- name: UpdateValidationRunProductionAttempt :exec
UPDATE validation_runs
SET production_operation_id = sqlc.arg(production_operation_id),
    production_version = sqlc.arg(production_version),
    production_attempt_no = sqlc.arg(production_attempt_no)
WHERE workspace_id = sqlc.arg(workspace_id)
  AND id = sqlc.arg(run_id);

-- name: ListValidationRunsForProductionAttempt :many
SELECT * FROM validation_runs
WHERE workspace_id = sqlc.arg(workspace_id)
  AND production_operation_id = sqlc.arg(production_operation_id)
  AND production_version = sqlc.arg(production_version)
  AND production_attempt_no = sqlc.arg(production_attempt_no)
ORDER BY started_at ASC, id ASC;

-- name: InsertProductionReviewBinding :exec
INSERT INTO production_review_bindings (
    workspace_id, review_id, operation_id, production_version, attempt_no,
    set_digest, validation_digest, created_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(review_id), sqlc.arg(operation_id), sqlc.arg(production_version),
    sqlc.arg(attempt_no), sqlc.arg(set_digest), sqlc.arg(validation_digest), sqlc.arg(created_at)
);

-- name: GetProductionReviewBinding :one
SELECT * FROM production_review_bindings
WHERE workspace_id = sqlc.arg(workspace_id)
  AND review_id = sqlc.arg(review_id);

-- name: ListProductionReviewBindingsForAttempt :many
SELECT * FROM production_review_bindings
WHERE workspace_id = sqlc.arg(workspace_id)
  AND operation_id = sqlc.arg(operation_id)
  AND production_version = sqlc.arg(production_version)
  AND attempt_no = sqlc.arg(attempt_no);

-- name: InsertReleaseProposal :exec
INSERT INTO release_proposals (
    workspace_id, release_id, proposal_id, operation_id, production_version,
    set_digest, proposal_content_digest, author_principal_id, attempt_no,
    validation_digest, review_ids, role, created_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(release_id), sqlc.arg(proposal_id), sqlc.arg(operation_id),
    sqlc.arg(production_version), sqlc.arg(set_digest), sqlc.arg(proposal_content_digest),
    sqlc.arg(author_principal_id), sqlc.arg(attempt_no), sqlc.arg(validation_digest),
    sqlc.arg(review_ids), sqlc.arg(role), sqlc.arg(created_at)
);

-- name: ListReleaseProposals :many
SELECT * FROM release_proposals
WHERE workspace_id = sqlc.arg(workspace_id) AND release_id = sqlc.arg(release_id)
ORDER BY created_at ASC, proposal_id ASC;

-- name: InsertProductionReleaseManifest :exec
INSERT INTO production_release_manifests (
    workspace_id, release_id, before_release_id, before_manifest_json,
    before_manifest_digest, after_manifest_digest, attribution_digest, created_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(release_id), sqlc.narg(before_release_id),
    sqlc.arg(before_manifest_json), sqlc.arg(before_manifest_digest),
    sqlc.arg(after_manifest_digest), sqlc.arg(attribution_digest), sqlc.arg(created_at)
);

-- name: GetProductionReleaseManifest :one
SELECT * FROM production_release_manifests
WHERE workspace_id = sqlc.arg(workspace_id) AND release_id = sqlc.arg(release_id);

-- name: InsertProductionReleaseBeforePin :exec
INSERT INTO production_release_before_pins (
    workspace_id, release_id, target_kind, target_id, presence,
    asset_revision_id, object_version, content_digest, created_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(release_id), sqlc.arg(target_kind), sqlc.arg(target_id),
    sqlc.arg(presence), sqlc.narg(asset_revision_id), sqlc.narg(object_version),
    sqlc.narg(content_digest), sqlc.arg(created_at)
);

-- name: ListProductionReleaseBeforePins :many
SELECT * FROM production_release_before_pins
WHERE workspace_id = sqlc.arg(workspace_id) AND release_id = sqlc.arg(release_id)
ORDER BY target_kind ASC, target_id ASC;

-- name: InsertProductionReleaseBindingInput :exec
INSERT INTO production_release_binding_inputs (
    workspace_id, release_id, binding_id, binding_version, snapshot_id,
    dataset_revision_id, field_revision_ids, created_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(release_id), sqlc.arg(binding_id), sqlc.arg(binding_version),
    sqlc.arg(snapshot_id), sqlc.arg(dataset_revision_id), sqlc.arg(field_revision_ids),
    sqlc.arg(created_at)
);

-- name: ListProductionReleaseBindingInputs :many
SELECT * FROM production_release_binding_inputs
WHERE workspace_id = sqlc.arg(workspace_id) AND release_id = sqlc.arg(release_id);

-- name: GetProductionTargetByProposalID :one
SELECT * FROM production_targets
WHERE workspace_id = sqlc.arg(workspace_id) AND proposal_id = sqlc.arg(proposal_id)
LIMIT 1;

-- name: CreateProductionRelease :one
INSERT INTO releases (
    id, workspace_id, sequence, manifest_digest, state, rolled_back_to_release_id,
    origin_proposal_id, published_by, published_at, created_at,
    production_root_release_id, production_rollback_depth, production_rollback_parent_id
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(sequence), sqlc.arg(manifest_digest),
    sqlc.arg(state), sqlc.narg(rolled_back_to_release_id), sqlc.narg(origin_proposal_id),
    sqlc.arg(published_by), sqlc.arg(published_at), sqlc.arg(created_at),
    sqlc.narg(production_root_release_id), sqlc.arg(production_rollback_depth), sqlc.narg(production_rollback_parent_id)
)
RETURNING *;

-- name: CreateProductionValidationRun :one
INSERT INTO validation_runs (
    id, workspace_id, proposal_id, validator_id, validator_version, status, started_at, finished_at,
    production_operation_id, production_version, production_attempt_no
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(proposal_id), sqlc.arg(validator_id),
    sqlc.arg(validator_version), sqlc.arg(status), sqlc.arg(started_at), sqlc.narg(finished_at),
    sqlc.arg(production_operation_id), sqlc.arg(production_version), sqlc.arg(production_attempt_no)
)
RETURNING *;

-- name: GetLatestRelease :one
SELECT * FROM releases
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY sequence DESC
LIMIT 1;
