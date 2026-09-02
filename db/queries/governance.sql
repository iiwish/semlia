-- name: CreateProposal :one
INSERT INTO proposals (
    id, workspace_id, asset_id, base_revision_id, target_object_type, target_object_id,
    state, title, summary, reason, created_by, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.narg(asset_id), sqlc.narg(base_revision_id),
    sqlc.arg(target_object_type), sqlc.arg(target_object_id), sqlc.arg(state), sqlc.arg(title),
    sqlc.arg(summary), sqlc.arg(reason), sqlc.arg(created_by), sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: GetProposal :one
SELECT * FROM proposals
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(proposal_id);

-- name: GetProposalForUpdate :one
SELECT * FROM proposals
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(proposal_id)
FOR UPDATE;

-- name: SubmitProposal :one
UPDATE proposals
SET state = 'proposed', submitted_at = sqlc.arg(submitted_at), updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(proposal_id) AND state = 'draft'
RETURNING *;

-- name: TransitionProposal :one
UPDATE proposals
SET state = sqlc.arg(state), decided_at = sqlc.narg(decided_at), updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(proposal_id)
  AND state = sqlc.arg(expected_state)
RETURNING *;

-- name: LinkProposalPolicyDecision :one
UPDATE proposals
SET policy_decision_id = sqlc.arg(policy_decision_id), risk_level = sqlc.arg(risk_level),
    updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(proposal_id)
RETURNING *;

-- name: CountProposalChanges :one
SELECT count(*) FROM proposal_changes WHERE proposal_id = sqlc.arg(proposal_id);

-- name: CreateProposalChange :one
INSERT INTO proposal_changes (
    id, workspace_id, proposal_id, field_path, op,
    before_digest, after_digest, before_value, after_value, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(proposal_id), sqlc.arg(field_path), sqlc.arg(op),
    sqlc.narg(before_digest), sqlc.narg(after_digest), sqlc.narg(before_value), sqlc.narg(after_value),
    sqlc.arg(created_at)
)
RETURNING *;

-- name: DeleteProposalChange :execrows
DELETE FROM proposal_changes
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(proposal_change_id)
  AND proposal_id = sqlc.arg(proposal_id);

-- name: ListProposalChanges :many
SELECT * FROM proposal_changes
WHERE workspace_id = sqlc.arg(workspace_id) AND proposal_id = sqlc.arg(proposal_id)
ORDER BY created_at, id;

-- name: CreateReview :one
INSERT INTO reviews (
    id, workspace_id, proposal_id, reviewer_principal_id, channel, decision, note, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(proposal_id), sqlc.arg(reviewer_principal_id),
    sqlc.arg(channel), sqlc.arg(decision), sqlc.arg(note), sqlc.arg(created_at)
)
RETURNING *;

-- name: CreateValidationRun :one
INSERT INTO validation_runs (
    id, workspace_id, proposal_id, validator_id, validator_version, status, started_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(proposal_id), sqlc.arg(validator_id),
    sqlc.arg(validator_version), sqlc.arg(status), sqlc.arg(started_at)
)
RETURNING *;

-- name: GetValidationRun :one
SELECT * FROM validation_runs
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(validation_run_id);

-- name: FinishValidationRun :one
UPDATE validation_runs
SET status = sqlc.arg(status), finished_at = sqlc.arg(finished_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(validation_run_id)
  AND status = 'running'
RETURNING *;

-- name: CreateValidationResult :one
INSERT INTO validation_results (
    id, workspace_id, validation_run_id, severity, code, message, input_digest, details, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(validation_run_id), sqlc.arg(severity),
    sqlc.arg(code), sqlc.arg(message), sqlc.arg(input_digest), sqlc.arg(details), sqlc.arg(created_at)
)
RETURNING *;

-- name: ListValidationRunResults :many
SELECT result.* FROM validation_results AS result
JOIN validation_runs AS run ON run.id = result.validation_run_id
WHERE result.workspace_id = sqlc.arg(workspace_id) AND run.id = sqlc.arg(validation_run_id)
ORDER BY result.created_at, result.id;

-- name: CountBlockingValidationResults :one
SELECT count(*) FROM validation_results AS result
JOIN validation_runs AS run ON run.id = result.validation_run_id
WHERE result.workspace_id = sqlc.arg(workspace_id)
  AND run.proposal_id = sqlc.arg(proposal_id)
  AND run.status IN ('running', 'succeeded')
  AND result.severity = 'blocker';

-- name: CreatePolicyDecision :one
INSERT INTO policy_decisions (
    id, workspace_id, proposal_id, rule_version, inputs, inputs_digest,
    matched_policy, risk_level, routing, reason_code, decided_at, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(proposal_id), sqlc.arg(rule_version),
    sqlc.arg(inputs), sqlc.arg(inputs_digest), sqlc.arg(matched_policy), sqlc.arg(risk_level),
    sqlc.arg(routing), sqlc.arg(reason_code), sqlc.arg(decided_at), sqlc.arg(created_at)
)
RETURNING *;

-- name: GetPolicyDecision :one
SELECT * FROM policy_decisions
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(policy_decision_id);

-- name: LockWorkspace :one
SELECT id FROM workspaces WHERE id = sqlc.arg(workspace_id) FOR UPDATE;

-- name: NextReleaseSequence :one
SELECT (COALESCE(max(sequence), 0) + 1)::bigint AS next_sequence FROM releases
WHERE workspace_id = sqlc.arg(workspace_id);

-- name: CreateRelease :one
INSERT INTO releases (
    id, workspace_id, sequence, manifest_digest, state, rolled_back_to_release_id,
    published_by, published_at, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(sequence), sqlc.arg(manifest_digest),
    sqlc.arg(state), sqlc.narg(rolled_back_to_release_id), sqlc.arg(published_by),
    sqlc.arg(published_at), sqlc.arg(created_at)
)
RETURNING *;

-- name: CreateReleaseAsset :exec
INSERT INTO release_assets (
    workspace_id, release_id, asset_id, revision_id, compatibility, position, created_at
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(release_id), sqlc.arg(asset_id), sqlc.arg(revision_id),
    sqlc.arg(compatibility), sqlc.arg(position), sqlc.arg(created_at)
);

-- name: GetRelease :one
SELECT * FROM releases
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(release_id);

-- name: GetPreviousRelease :one
SELECT * FROM releases
WHERE workspace_id = sqlc.arg(workspace_id) AND sequence < sqlc.arg(sequence)
ORDER BY sequence DESC
LIMIT 1;

-- name: CountReleaseRollbacks :one
SELECT count(*) FROM releases
WHERE workspace_id = sqlc.arg(workspace_id) AND rolled_back_to_release_id = sqlc.arg(release_id);

-- name: ListReleaseAssets :many
SELECT * FROM release_assets
WHERE workspace_id = sqlc.arg(workspace_id) AND release_id = sqlc.arg(release_id)
ORDER BY position;

-- name: GetAssetRevisionOwnership :one
SELECT asset_id FROM asset_revisions
WHERE workspace_id = sqlc.arg(workspace_id) AND asset_id = sqlc.arg(asset_id)
  AND id = sqlc.arg(revision_id);

-- name: CreateAgentRun :one
INSERT INTO agent_runs (
    id, workspace_id, principal_id, model, config_revision, input_hash, status,
    cost_micros, started_at, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.narg(principal_id), sqlc.arg(model),
    sqlc.arg(config_revision), sqlc.arg(input_hash), sqlc.arg(status), sqlc.arg(cost_micros),
    sqlc.arg(started_at), sqlc.arg(created_at)
)
RETURNING *;

-- name: GetAgentRun :one
SELECT * FROM agent_runs
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(agent_run_id);

-- name: FinishAgentRun :one
UPDATE agent_runs
SET status = sqlc.arg(status), output_digest = sqlc.narg(output_digest),
    cost_micros = sqlc.arg(cost_micros), finished_at = sqlc.arg(finished_at),
    duration_ms = sqlc.arg(duration_ms)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(agent_run_id)
  AND status = 'running'
RETURNING *;

-- name: CreateAgentStep :one
INSERT INTO agent_steps (
    id, workspace_id, agent_run_id, sequence, kind, tool_name,
    input_hash, output_hash, error_code, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(agent_run_id), sqlc.arg(sequence), sqlc.arg(kind),
    sqlc.narg(tool_name), sqlc.arg(input_hash), sqlc.arg(output_hash), sqlc.narg(error_code),
    sqlc.arg(created_at)
)
RETURNING *;
