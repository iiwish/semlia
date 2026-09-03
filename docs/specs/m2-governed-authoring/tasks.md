# M2 Governed Authoring Work Graph

## Status

- `Ready`: dependencies, checklist, analysis and packet are complete.
- `Running`: implementation is active.
- `Needs_Review`: implementation and evidence pass; user acceptance remains.
- `Accepted`: explicitly accepted or included in an accepted milestone.
- `Blocked`: a named condition prevents safe progress.
- `Superseded`: a later founder decision replaces the task's product direction.

## Work Units

### T001 Authorization foundation

Status: Accepted
Depends on: M1 Accepted, D6/D7 verdicts recorded 2026-09-02
Blocks: T002, T006

Introduce human and agent principals, the nine FR-004 system roles, role bindings with scopes,
deny-by-default capability enforcement on protected commands, and immutable authorization events.
Backend enforcement replaces frontend-only capability simulation; no product write path ships
without it.

Packet: `docs/specs/m2-governed-authoring/packets/T001.yaml`.

Execution: implementation `cdba99a`; deny-by-default evaluation with full decision audit, the four
packet red scenarios, real-PostgreSQL integration suites and the complete validation set pass.
Evidence: `docs/evidence/M2-T001/summary.md`.

Acceptance: founder explicitly accepted T001 on 2026-09-02.

### T002 Proposal and governance data model

Status: Accepted
Depends on: T001
Blocks: T003, T008, T009

Add the proposal, change-set, review, validation run, policy decision, release, and agent run
schema with the SSOT §7.5 state machine, following the M1 transaction and immutability patterns.

Packet: `docs/specs/m2-governed-authoring/packets/T002.yaml`.

Execution: implementation `e2adaf9`; the ten-table model, domain transition table, concurrent
immutability attacks, policy recomputability, populated-migration lifecycle and atomic
audit+outbox commits pass the complete source gate. Evidence:
`docs/evidence/M2-T002/summary.md`.

Acceptance: founder explicitly accepted T001 and T002 on 2026-09-02 and authorized continuous M2
execution through the T007 backend governed loop without per-task approval pauses, with T009
parallelized once T003 and T006 are delivered.

### T003 Proposal API and AI output contract

Status: Accepted
Depends on: T002, T008
Blocks: T004, T009

Condition: packet authored after T002 acceptance. Generated OpenAPI contracts for proposal
create/list/detail with structured patches; `github.com/santhosh-tekuri/jsonschema/v6` validates AI
structured output before domain entry; agent attribution references agent runs; no-substantive-change
scan keeps empty diffs out of the human queue.

Packet: `docs/specs/m2-governed-authoring/packets/T003.yaml`.

Execution: implementation adds the governance proposal API surface (list/create/detail/submit)
under `/api/v1/workspaces/{workspaceId}/governance/proposals` with regenerated contracts, the
versioned AI proposal-input JSON Schema gate (422 with zero domain writes), agent-run attribution
matching, the no-substantive-change gate and server-side capability enforcement. Evidence:
`docs/evidence/M2-T003/summary.md`.

Acceptance: included in the founder's continuous M2 execution authorization on 2026-09-02.

### T004 Validation orchestration

Status: Blocked
Depends on: T003
Blocks: T005

Condition: packet authored after T003 acceptance. Deterministic validator registry (schema,
reference, structural in v1) running as jobs; runs record validator version, input digest and
results; validator failure degrades to human handling without silent pass.

### T005 Risk assessment and policy decisions

Status: Blocked
Depends on: T004
Blocks: T006

Condition: packet authored after T004 acceptance. Recomputable risk inputs per SSOT §8.3,
versioned rule-table policy decisions, explainable routing to expert and batch lanes; no auto
channel and no single opaque score.

### T006 Review workbench and separation of duties

Status: Blocked
Depends on: T001, T005
Blocks: T007, T010

Condition: packet authored after T005 acceptance. Expert approve/reject with reasons, batch
confirmation with clustering, sampling, exclusions and full batch audit, mid-batch high-risk
auto-split, server-side FR-007 enforcement including reviewer≠publisher on protected scopes.

### T007 Release and rollback

Status: Blocked
Depends on: T006
Blocks: T010, T011

Condition: packet authored after T006 acceptance. Immutable release manifest, current-revision
switch, Git projection of released content, rollback as a new release pointing at a prior revision
per decision D8. The packet must forbid tags, remotes and history rewriting in the Git projection.

### T008 Governance objects

Status: Accepted
Depends on: T002
Blocks: T003

Condition: packet authored after T002 acceptance. ModelGrain, EntityKey, JoinContract and
PhysicalBinding domain, migrations and proposal support on the object-generic change-set pipeline.

Packet: `docs/specs/m2-governed-authoring/packets/T008.yaml`.

Execution: implementation adds migration `000007`, the four object domains with structural
validation, submission-time target existence, governed change application with version bump and
audit facts, and the populated-migration lifecycle to version 7. Evidence:
`docs/evidence/M2-T008/summary.md`.

Acceptance: included in the founder's continuous M2 execution authorization on 2026-09-02.

### T009 Model configuration and live LLM generation

Status: Blocked
Depends on: T003, T006, D1 verdict recorded 2026-09-02
Blocks: T010

Condition: packets authored after T006 acceptance. Persisted LLM/Embedding
provider and model settings honoring the ModelConfigurationView contract; live schema-valid
proposal generation; full §8.6 run recording. The governed loop stays provable on seeded
agent-attributed proposals if provider integration slips.

### T010 Frontend convergence

Status: Blocked
Depends on: T007, T009
Blocks: T011

Condition: packet authored after T007/T009 acceptance. Proposal authoring, review, release and
rollback views use real M2 APIs at the catalogRuntime seam; Ask and other future-milestone surfaces
remain preview or session-only with visible disclosure, and production request failures never fall
back to fixture data.

### T011 Production acceptance

Status: Blocked
Depends on: T007, T010
Blocks: M2 milestone acceptance

Condition: packet authored after T010 acceptance. Exact-ref journey covering propose (seeded and
live LLM) → validate → review → publish → rollback, golden recompute cases, migration/recovery
suites, contract drift, security/release gates and independent reviews.
