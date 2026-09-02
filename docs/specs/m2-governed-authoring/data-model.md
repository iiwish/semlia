# M2 Governed Authoring Data Model

## Storage Contract

PostgreSQL remains the authority for governance state; Git stores projected released content only
(SSOT D-003). All identifiers are UUIDv7 with TypeIDs on the wire per ADR-0003
(`prp_`, `chg_`, `rvw_`, `val_`, `rls_`, `agr_` prefixes proposed at contract time). Table and
column naming aligns with the SSOT §11.2 core table group. Status dimensions stay orthogonal per
`docs/specs/semantic-assets/semantic-asset-design.md` §4.8: workflow state lives on proposals,
lifecycle state on assets, deployment state on environments — never one enum.

## Aggregate Map

| Aggregate | Core tables | Invariant |
| --- | --- | --- |
| Principal / authorization | `principals`, `roles`, `role_bindings`, `authorization_events` | Deny-by-default; agent principals reference their workspace; every protected command writes an authorization decision event |
| Proposal | `proposals`, `proposal_changes`, `reviews` | Patch is a structured change-set against a released baseline revision; state machine follows SSOT §7.5; `proposal_changes` rows are immutable once submitted |
| Validation run | `validation_runs`, `validation_results` | One run per proposal per validator version; results record tool version, input digest, outcome; runs never mutate revisions |
| Release | `releases`, `release_assets` | Manifest is immutable; entries pin `asset_id + revision_id` and compatibility conclusion; rollback creates a new release row, never updates or deletes one |
| Agent run | `agent_runs`, `agent_steps` | Every AI write references one agent run; runs record model, config revision, input hash, tool calls, output digest, cost, duration, final state (SSOT §8.6) |

## Key Shapes

### Authorization

`principals(id, workspace_id, kind human|agent, display_name, status, created_at)` — agent
principals carry `owner_principal_id` for accountability. `role_bindings(id, principal_id, role_id,
scope_type, scope_id, granted_by, granted_at)` mirrors the frontend binding model; `roles` seeds the
nine FR-004 system roles. Protected commands evaluate capabilities server-side and append
`authorization_events` (decision, reason code, authorization version).

### Proposal

`proposals(id, workspace_id, asset_id, base_revision_id, target_object_type, target_object_id,
state, title, summary, reason, risk_level, policy_decision_id, agent_run_id, created_by,
submitted_at, decided_at)` — `state` walks `draft → proposed → validating → in_review → released |
rejected` with transitions validated in the domain layer. `proposal_changes(id, proposal_id,
field_path, op, before_digest, after_digest, before_value jsonb, after_value jsonb)` carries the
structured patch; digests make diffs verifiable without storing full baselines.

### Validation

`validation_runs(id, proposal_id, validator_id, validator_version, status, started_at, finished_at)`
and `validation_results(id, run_id, severity, code, message, input_digest, details jsonb)` —
`Blocker` severity blocks release candidacy per the AssetTypeProfile severity enum
(`docs/specs/semantic-assets/semantic-asset-design.md` §3.3).

### Release

`releases(id, workspace_id, manifest_digest, state, published_by, published_at, rolled_back_to_release_id)`,
`release_assets(release_id, asset_id, revision_id, compatibility jsonb, position)` — immutability is
enforced by trigger-free repository invariants (update/delete denied), mirroring the M1 revision
pattern in `internal/adapters/postgres/catalog.go`.

### Policy and risk

`policy_decisions(id, proposal_id, rule_version, inputs jsonb, inputs_digest, matched_policy,
risk_level, routing channel expert|batch, reason_code, decided_at)` — identical `inputs` must
reproduce the identical decision (SSOT §8.2); `inputs_digest` makes recomputation testable.

## Migration Strategy

1. `000005_m2_authorization_foundation` — principals, roles, role_bindings, authorization_events.
2. `000006_m2_governed_authoring` — proposals, proposal_changes, reviews, validation_runs,
   validation_results, policy_decisions, releases, release_assets, agent_runs, agent_steps.
3. `000007_m2_governance_objects` — ModelGrain, EntityKey, JoinContract and PhysicalBinding
   persistence, plus proposal references to the four object types.
4. Downgrades drop M2 tables without touching M0/M1 rows; populated-upgrade and
   rollback/upgrade suites follow the M1 migration test pattern.

## Index And Search Baseline

- Catalog search reuse: PostgreSQL FTS and `pg_trgm` remain the baseline; proposal and release
  listings are workspace-scoped keyset-paginated like M1 catalog lists.
- Recomputability indexes: `policy_decisions(inputs_digest)`, `validation_runs(proposal_id,
  validator_id)`.
- Audit joins: `authorization_events(principal_id, created_at)`,
  `agent_runs(workspace_id, created_at)` for §8.6 traceability queries.
