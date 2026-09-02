# M2 Planning Analysis

## Decision Register

All eight decisions were confirmed per recommendation by the founder on 2026-09-02. Each row cites
its SSOT basis; `docs/README.md` 文档规则 #2 forbids restating the clauses here. Execution
authorization for T001/T002 remains a separate founder decision.

| ID | Decision | Options | Confirmed decision | Basis |
| --- | --- | --- | --- | --- |
| D1 | AI supply for agent runs | (a) provider API with persisted model-config backend; (b) self-hosted model first; (c) defer live LLM | (a), with the governed loop provable on agent-attributed seeded proposals before live LLM lands (T009 off the critical path) | SSOT §8.6; ModelConfigurationView contract |
| D2 | First slice width | (a) thin vertical loop (propose→validate→review→publish→rollback) then deepen; (b) validator framework first | (a); the change-set schema stays object-generic from day one so the four governance objects join without rework | SSOT §17 M2; ADR-0002 §3 |
| D3 | Policy engine depth in v1 | (a) versioned rule-table evaluator in Go; (b) adopt `cel-go` now | (a). The TDR-015 condition "policy language 与审计语义 Confirmed" is not yet met; adoption reopens on founder confirmation | ADR-0002 TDR-015; SSOT §8.2 |
| D4 | Ask surface in M2 | (a) upgrade Ask to ResolvedSemanticPlan-driven; (b) keep preview until M3 | (b). Resolution belongs to M3; M2 records this boundary explicitly | D-015; SSOT §17 M3 |
| D5 | Chinese display-name search | (a) fold into M2; (b) small standalone fix; (c) defer | Verify first: M1 documents address-prefix + full-text search (`docs/specs/m1-semantic-registry/t004-api-design.md`), so the T009 observation is likely PostgreSQL FTS tokenization, not scope. Track as a standalone small task, not an M2 scope driver | M1 T004 design; T009 acceptance observation |
| D6 | Agent actor identity | (a) agent principals as workspace-scoped service principals with distinct identity and permissions; (b) agent writes attributed to a human operator | (a). SSOT §8.6 requires agent runs to be attributable; permissions stay deny-by-default and agents cannot self-promote release levels | SSOT §8.6, §8.1; FR-003 |
| D7 | Separation of duties under private incubation | (a) role-separated founder accounts (steward vs publisher); (b) seed a second reviewer principal; (c) suspend FR-007 in M2 | (a) or (b) — FR-007 stays enforced server-side; the founder must pick the mechanism. Suspendcing SoD would contradict FR-007 and SSOT §8.4 | FR-007; SSOT §8.3 |
| D8 | Rollback semantics | (a) rollback as a new immutable release pointing at the prior revision; (b) revert/release deletion | (a). Preserves P-003 immutability and audit; Git history is append-only | P-003; SSOT §8.2 |

## Cross-Contract Checks

| Concern | Resolution |
| --- | --- |
| M1 revisions become current on append; no draft/review state exists | The proposal state machine layers on top; publish switches the current revision via the existing `SetCurrentAssetRevision` invariant. M1 revision storage, evidence linking and digests are reused unchanged. |
| Backend has no actor identity or authorization | T001 introduces principals, capability enforcement and audit before any M2 write path exists; frontend `authorization.tsx` and `PermissionAction` are the contract, not the implementation. |
| Governance objects (PhysicalBinding, ModelGrain, EntityKey, JoinContract) have no M1 schema | T008 adds the domain and migrations; proposals reference them through the object-generic change-set from T002. |
| Outbox envelope is strict (`DisallowUnknownFields`) | New M2 event types (`proposal.*`, `release.*`) extend the `semlia.events/v1` envelope contract with the same validation discipline. |
| Web fixtures already model proposals, releases, validation gates | `web/src/types.ts` and `data.ts` fixtures are the UI-side spec; T008 replaces them with API data at the `catalogRuntime.tsx` seam. Preview labels remain for surfaces without backend milestones. |
| LLM/embedding configuration is session-only | T006 persists the `ModelConfigurationView` contract; the embedding rebuild run stays simulated until its milestone. |

## Risks

- T001 must land before any M2 write path; shipping proposal APIs on free-text `CreatedBy` would
  violate SSOT §8.6 traceability and FR-011 server-side authorization.
- Policy and risk records must be recomputable from persisted inputs from day one; retrofitting
  recomputability after the review workbench exists would invalidate audit history.
- The batch channel must never mix high-risk items; escalation auto-split needs its own tests, not
  just clustering tests.
- Rollback tests must prove release immutability (no update-in-place, no deletion) under
  concurrency, mirroring the M1 `AppendCatalogRevision` invariant discipline.
- The four governance objects expand the validator matrix; the thin slice ships with schema,
  reference and structural validators, and the validator registry must make later validators
  additive without contract breaks.
- Live LLM integration depends on external providers; seeded agent-attributed proposals keep
  AC-M2-001..009 provable when providers are unavailable (D1).
