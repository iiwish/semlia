# M2 Governed AI Authoring Specification

## Metadata

| Field | Value |
| --- | --- |
| Milestone | M2 Governed AI Authoring |
| Version | 0.1.0 |
| Status | Confirmed — decision register D1–D8 confirmed by founder on 2026-09-02 |
| Scope basis | `docs/SSOT.md` §17 M2, §18 首个公开版本范围 |
| Product contract | `docs/SSOT.md` §8 治理流水线, §7.5 生命周期, P-002/P-003/P-011 |
| Technical direction | `docs/adr/0002-m1-product-and-semantic-execution.md` §3 M1 excludes, TDR-013/014/015 |
| Authorization | Founder accepted T009/M1 and authorized M2 planning on 2026-09-02; execution authorization is a separate founder decision |
| Completion evidence | `docs/evidence/M2-T0NN/summary.md` per task |

## Outcome

M2 lets AI participate safely in semantic engineering inside the accepted production frontend: an
agent-attributed proposal carries a structured patch against the released baseline, deterministic
validators gate it, a versioned policy decision routes it to the batch or expert channel, a human
review decides, publishing cuts an immutable release manifest, and rollback restores a prior
revision as a new release. The loop "AI 提案、验证、人类审核、发布、回滚" (SSOT §17 M2 exit
criterion) completes on real PostgreSQL data through generated contracts, with every AI write
traceable per SSOT §8.6.

M2 does not introduce the auto-publish channel, G2/G3 levels, shadow evaluation, consumer
bindings, MCP delivery, or trusted resolution. Those belong to M4/M5 and M3 respectively (SSOT
§17). Ask remains a preview surface in M2.

## Required Capabilities

- Actor identity and authorization enforcement: human and agent principals, deny-by-default
  capability checks on every protected command, authorization audit events (FR-003/FR-007/NFR-001
  of `docs/specs/access-control/product-design.md`).
- Proposal and change-set model: structured patches relative to the released baseline, proposal
  state machine per SSOT §7.5, agent attribution (agent run, model, config, input hash, cost,
  duration).
- Validation orchestration: deterministic validator registry and runs on the existing job
  framework, recording tool version, input digest and results (SSOT §13 NFR-003); validator failure
  degrades to human handling, never silent pass (SSOT §8.2).
- Change-level risk assessment: recomputable on identical inputs, versioned policy decisions,
  explainable routing reasons; never a single opaque score (SSOT §8.3).
- Review channels: expert review and batch confirmation with clustering, sampling, escalation
  auto-split and full batch audit (SSOT §8.4); separation of duties enforced server-side
  (FR-007).
- Publishing and rollback: immutable release manifest, current-revision switch, Git projection of
  released content, rollback expressed as a new release pointing at a prior revision (P-003,
  SSOT §8.2).
- Governance object coverage: proposal, validation and release support for PhysicalBinding,
  ModelGrain, EntityKey and JoinContract on the same change-set pipeline (SSOT §17 M2).
- Model configuration backend: persisted LLM/Embedding provider and model settings honoring the
  `ModelConfigurationView` contract, so live AI proposal generation is reproducible.
- Frontend convergence: proposal authoring, review, release and rollback views use real M2 APIs;
  surfaces without backend milestones stay preview or session-only (ADR-0002 §7).

## Acceptance Scenarios

| ID | Scenario | Required proof |
| --- | --- | --- |
| AC-M2-001 | An agent-attributed proposal with a structured patch against the released baseline passes schema validation and enters `proposed`. | API test: malformed AI output rejected before domain entry; valid patch persisted with agent run reference. |
| AC-M2-002 | Deterministic validators execute as jobs and record validator version, input digest and results; a failing validator routes the proposal to human handling. | Job and repository tests; no silent-pass path. |
| AC-M2-003 | Risk assessment and the policy decision are recomputable on identical inputs and record rule version, inputs, matched policy and routing reason. | Deterministic recompute test; golden case per change family. |
| AC-M2-004 | Expert review approves or rejects with reasons; reviewer and publisher roles are separated server-side on protected scopes. | AuthZ integration test asserting FR-007 conflicts return stable reason codes. |
| AC-M2-005 | A batch confirmation persists grouping rule, samples, exclusions, reviewer and policy version; a mid-batch high-risk item splits out automatically. | Batch audit test; escalation split test. |
| AC-M2-006 | Publishing cuts an immutable release manifest and switches the current revision; the Git projection reflects the release. | Repository, projection and immutability tests. |
| AC-M2-007 | Rollback creates a new release pointing at a prior revision; release, audit and usage signals record the rollback. | Rollback journey test; audit/outbox assertions. |
| AC-M2-008 | A G0 asset accepts AI drafts only and requires the designated owner to publish; a new workspace defaults to G1. | Policy routing test per SSOT §8.1. |
| AC-M2-009 | Every agent run records model, config, input hash, tool calls, output, cost, duration and final state. | Agent run persistence test per SSOT §8.6. |
| AC-M2-010 | With a persisted model configuration, a live LLM generates a schema-valid proposal end to end. | Contract test against the configured provider; seeded fallback keeps AC-M2-001..009 independent of provider availability. |

## Boundaries

- Release levels ship G0 and G1 only; new workspaces default to G1; G2/G3 require explicit admin
  enablement that M2 does not implement (SSOT §8.1, §17).
- The auto channel of SSOT §8.4 ships in M4; M2 builds the risk and policy data structures (rule
  version, evidence window, routing reason) that M4 reuses, without auto-publish.
- JSON Schema validation adopts `github.com/santhosh-tekuri/jsonschema/v6` — the TDR-015 adoption
  condition "校验 AI structured output" is met by this specification.
- CEL (`github.com/google/cel-go`) is not adopted in M2 v1; the TDR-015 condition "policy language
  与审计语义 Confirmed" is not yet met. V1 policy evaluation is a versioned rule-table evaluator in
  Go. The decision reopens when the founder confirms the condition.
- CodeMirror enters only if the founder confirms a structured or YAML editor scope (TDR-013).
- Optional Cube compile validation extends the existing `CubeAdapter` contract (TDR-014); no new
  integration path.
- The Git projection writer projects release state; it does not gain tags, remotes or history
  rewriting (ADR-0002; P-005).
- `web` remains the sole production frontend; no fixture fallback on production request failure
  (ADR-0002 §7).

## Non-Goals

- Auto channel, shadow evaluation, G1→G2/G3 promotion, sampling audit of auto-approved items:
  belong to M4/M5 (SSOT §17).
- Trusted resolution, ResolvedSemanticPlan enforcement, MCP, CLI/SDK delivery and consumer
  bindings: belong to M3 (SSOT §17, D-015).
- Agent quality evaluation sets and drift dashboards: belong to M4.
- Public release: blocked on §15.4 gates, unchanged by M2.
