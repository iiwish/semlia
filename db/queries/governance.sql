-- name: CreateProposal :one
INSERT INTO proposals (
    id, workspace_id, asset_id, base_revision_id, target_object_type, target_object_id,
    state, title, summary, reason, agent_run_id, created_by, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.narg(asset_id), sqlc.narg(base_revision_id),
    sqlc.arg(target_object_type), sqlc.arg(target_object_id), sqlc.arg(state), sqlc.arg(title),
    sqlc.arg(summary), sqlc.arg(reason), sqlc.narg(agent_run_id), sqlc.arg(created_by),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: GetProposal :one
SELECT * FROM proposals
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(proposal_id);

-- name: ListProposals :many
SELECT * FROM proposals
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (
      NOT sqlc.arg(has_cursor)::boolean
      OR created_at < sqlc.arg(cursor_created_at)
      OR (created_at = sqlc.arg(cursor_created_at) AND id < sqlc.arg(cursor_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

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

-- name: CountProposalReviewsByReviewer :one
SELECT count(*) FROM reviews
WHERE workspace_id = sqlc.arg(workspace_id) AND proposal_id = sqlc.arg(proposal_id)
  AND reviewer_principal_id = sqlc.arg(reviewer_principal_id)
  AND channel = sqlc.arg(channel);

-- ---------- review batches (SSOT §8.4 batch confirmation channel) ----------

-- name: CreateReviewBatch :one
INSERT INTO review_batches (
    id, workspace_id, grouping_rule, policy_version, status, created_by, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(grouping_rule), sqlc.arg(policy_version),
    'open', sqlc.arg(created_by), sqlc.arg(created_at)
)
RETURNING *;

-- name: CreateReviewBatchMember :exec
INSERT INTO review_batch_members (
    review_batch_id, workspace_id, proposal_id, added_reason, sample, created_at
) VALUES (
    sqlc.arg(review_batch_id), sqlc.arg(workspace_id), sqlc.arg(proposal_id),
    sqlc.arg(added_reason), sqlc.arg(sample), sqlc.arg(created_at)
);

-- name: GetReviewBatch :one
SELECT * FROM review_batches
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(review_batch_id);

-- name: GetReviewBatchForUpdate :one
SELECT * FROM review_batches
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(review_batch_id)
FOR UPDATE;

-- name: ListOpenReviewBatches :many
SELECT * FROM review_batches
WHERE workspace_id = sqlc.arg(workspace_id) AND status = 'open'
  AND (
      NOT sqlc.arg(has_cursor)::boolean
      OR created_at < sqlc.arg(cursor_created_at)
      OR (created_at = sqlc.arg(cursor_created_at) AND id < sqlc.arg(cursor_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListReviewBatchMembers :many
SELECT * FROM review_batch_members
WHERE workspace_id = sqlc.arg(workspace_id) AND review_batch_id = sqlc.arg(review_batch_id)
ORDER BY created_at, proposal_id;

-- Locks the member rows together with their proposals so the confirm command
-- re-checks and applies outcomes over stable state (no concurrent review or
-- transition can interleave between the check and the write).
-- name: LockReviewBatchMembersWithProposals :many
SELECT member.review_batch_id, member.workspace_id, member.proposal_id, member.added_reason,
    member.decision, member.sample, member.split_out, member.split_reason, member.created_at,
    proposal.state AS proposal_state, proposal.created_by AS proposal_created_by,
    proposal.title AS proposal_title,
    proposal.target_object_type AS proposal_target_type,
    proposal.target_object_id AS proposal_target_object_id,
    proposal.created_at AS proposal_created_at
FROM review_batch_members AS member
JOIN proposals AS proposal
  ON proposal.workspace_id = member.workspace_id AND proposal.id = member.proposal_id
WHERE member.workspace_id = sqlc.arg(workspace_id) AND member.review_batch_id = sqlc.arg(review_batch_id)
ORDER BY member.created_at, member.proposal_id
FOR UPDATE OF member, proposal;

-- name: UpdateReviewBatchMemberOutcome :exec
UPDATE review_batch_members
SET decision = sqlc.narg(decision), sample = sqlc.arg(sample), split_out = sqlc.arg(split_out),
    split_reason = sqlc.narg(split_reason)
WHERE workspace_id = sqlc.arg(workspace_id) AND review_batch_id = sqlc.arg(review_batch_id)
  AND proposal_id = sqlc.arg(proposal_id);

-- name: ConfirmReviewBatch :one
UPDATE review_batches
SET status = sqlc.arg(status), decided_by = sqlc.arg(decided_by), decided_at = sqlc.arg(decided_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(review_batch_id)
  AND status = 'open'
RETURNING *;

-- One deterministic eligibility scan for batch assembly: in_review proposals
-- whose LATEST policy decision routes to the batch channel and that are not
-- already an active member of an open batch.
-- name: ListBatchEligibleProposals :many
SELECT proposal.*,
    decision.matched_policy AS decision_matched_policy,
    decision.risk_level AS decision_risk_level,
    decision.routing AS decision_routing,
    decision.reason_code AS decision_reason_code,
    decision.rule_version AS decision_rule_version,
    decision.inputs AS decision_inputs,
    decision.inputs_digest AS decision_inputs_digest
FROM proposals AS proposal
JOIN LATERAL (
    SELECT pd.matched_policy, pd.risk_level, pd.routing, pd.reason_code,
           pd.rule_version, pd.inputs, pd.inputs_digest
    FROM policy_decisions AS pd
    WHERE pd.proposal_id = proposal.id
    ORDER BY pd.decided_at DESC, pd.id DESC
    LIMIT 1
) AS decision ON true
WHERE proposal.workspace_id = sqlc.arg(workspace_id)
  AND proposal.state = 'in_review'
  AND decision.routing = 'batch'
  AND NOT EXISTS (
      SELECT 1 FROM review_batch_members AS member
      JOIN review_batches AS batch ON batch.id = member.review_batch_id
      WHERE member.proposal_id = proposal.id
        AND member.split_out = false
        AND batch.status = 'open'
  )
ORDER BY proposal.created_at, proposal.id;

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
ON CONFLICT (proposal_id, rule_version, inputs_digest) DO NOTHING
RETURNING *;

-- name: GetProposalPolicyDecisionByVersionDigest :one
SELECT * FROM policy_decisions
WHERE workspace_id = sqlc.arg(workspace_id) AND proposal_id = sqlc.arg(proposal_id)
  AND rule_version = sqlc.arg(rule_version) AND inputs_digest = sqlc.arg(inputs_digest);

-- name: GetLatestProposalPolicyDecision :one
SELECT * FROM policy_decisions
WHERE workspace_id = sqlc.arg(workspace_id) AND proposal_id = sqlc.arg(proposal_id)
ORDER BY decided_at DESC, id DESC
LIMIT 1;

-- name: ListPolicyRules :many
SELECT * FROM policy_rules
WHERE rule_version = sqlc.arg(rule_version)
ORDER BY priority DESC, rule_id;

-- name: GetPolicyRule :one
SELECT * FROM policy_rules
WHERE rule_version = sqlc.arg(rule_version) AND rule_id = sqlc.arg(rule_id);

-- name: SummarizeProposalValidationOutcomes :one
SELECT
    (SELECT count(*) FROM validation_runs AS run
      WHERE run.workspace_id = sqlc.arg(workspace_id) AND run.proposal_id = sqlc.arg(proposal_id)) AS run_count,
    (SELECT count(*) FROM validation_runs AS run
      WHERE run.workspace_id = sqlc.arg(workspace_id) AND run.proposal_id = sqlc.arg(proposal_id)
        AND run.status = 'failed') AS failed_count,
    (SELECT count(*) FROM validation_results AS result
      JOIN validation_runs AS run ON run.id = result.validation_run_id
      WHERE result.workspace_id = sqlc.arg(workspace_id) AND run.proposal_id = sqlc.arg(proposal_id)
        AND result.severity = 'blocker') AS blocker_count,
    (SELECT count(*) FROM validation_results AS result
      JOIN validation_runs AS run ON run.id = result.validation_run_id
      WHERE result.workspace_id = sqlc.arg(workspace_id) AND run.proposal_id = sqlc.arg(proposal_id)
        AND result.severity = 'warning') AS warning_count,
    (SELECT count(*) FROM validation_results AS result
      JOIN validation_runs AS run ON run.id = result.validation_run_id
      WHERE result.workspace_id = sqlc.arg(workspace_id) AND run.proposal_id = sqlc.arg(proposal_id)
        AND result.severity = 'info') AS info_count;

-- name: GetPolicyAssetFacts :one
SELECT asset.asset_type AS asset_type,
    (SELECT count(*) FROM revision_evidence_links AS link
       JOIN asset_revisions AS revision
         ON revision.workspace_id = link.workspace_id AND revision.id = link.asset_revision_id
      WHERE revision.asset_id = asset.id) AS evidence_link_count,
    current_revision.content AS current_content
FROM semantic_assets AS asset
LEFT JOIN asset_revisions AS current_revision
       ON current_revision.workspace_id = asset.workspace_id
      AND current_revision.id = asset.current_revision_id
WHERE asset.workspace_id = sqlc.arg(workspace_id) AND asset.id = sqlc.arg(asset_id);

-- name: GetPolicyGovernedObjectAsset :one
SELECT binding.asset_id FROM physical_bindings AS binding
  WHERE binding.workspace_id = sqlc.arg(workspace_id) AND binding.id = sqlc.arg(object_id)
UNION ALL
SELECT grain.asset_id FROM model_grains AS grain
  WHERE grain.workspace_id = sqlc.arg(workspace_id) AND grain.id = sqlc.arg(object_id)
UNION ALL
SELECT entity_key.asset_id FROM entity_keys AS entity_key
  WHERE entity_key.workspace_id = sqlc.arg(workspace_id) AND entity_key.id = sqlc.arg(object_id)
LIMIT 1;

-- name: GetPolicyRevisionOwnerContent :one
SELECT content FROM asset_revisions
WHERE workspace_id = sqlc.arg(workspace_id) AND asset_id = sqlc.arg(asset_id)
  AND id = sqlc.arg(revision_id);


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

-- name: GetProposalValidationRun :one
SELECT * FROM validation_runs
WHERE workspace_id = sqlc.arg(workspace_id) AND proposal_id = sqlc.arg(proposal_id)
  AND validator_id = sqlc.arg(validator_id)
  AND validator_version = sqlc.arg(validator_version);

-- name: ListProposalValidationRuns :many
SELECT * FROM validation_runs
WHERE workspace_id = sqlc.arg(workspace_id) AND proposal_id = sqlc.arg(proposal_id)
ORDER BY started_at, validator_id;

-- name: SemanticAssetExists :one
SELECT EXISTS (
    SELECT 1 FROM semantic_assets
    WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(asset_id)
) AS present;

-- name: AssetRevisionExists :one
SELECT EXISTS (
    SELECT 1 FROM asset_revisions
    WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(revision_id)
) AS present;

-- name: PhysicalDatasetExists :one
SELECT EXISTS (
    SELECT 1 FROM physical_datasets
    WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(physical_dataset_id)
) AS present;

-- name: PhysicalFieldExists :one
SELECT EXISTS (
    SELECT 1 FROM physical_fields
    WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(physical_field_id)
) AS present;
