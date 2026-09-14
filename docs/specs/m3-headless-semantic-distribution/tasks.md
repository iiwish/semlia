# M3 Headless Semantic Distribution Work Graph

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Confirmed |
| Plan | `docs/specs/m3-headless-semantic-distribution/plan.md` 0.1.0 Confirmed |
| Last updated | 2026-09-04 |

## Work Units

### M3-T001 Distribution contracts and release snapshot

Status: Needs_Review
Delivery profile: Alpha 0.1.0
Parent execution: Alpha T005
Depends on: Alpha T001 and T003 evidence complete; M2 immutable release path present
Blocks: M3-T002

Add canonical SemanticQuery, ResolvedSemanticPlan, query validation and refusal contracts; TypeIDs;
distribution persistence; and the repository projection that reads exact asset revisions and
governance object versions from one selected release. No candidate matching or public mutation is
implemented before the release projection proves cross-workspace and version isolation.

Proof: generated contract drift, populated migration, immutable projection tests and current/
explicit/pinned release selection cases.

Packet: authored inside the Alpha T005 execution packet after technical-plan approval and dependency
evidence.

### M3-T002 Deterministic resolver and plan validators

Status: Needs_Review
Delivery profile: Alpha 0.1.0
Parent execution: Alpha T005
Depends on: M3-T001 evidence complete
Blocks: M3-T003

Implement versioned exact/address/name/alias/lexical matching, authorization-safe candidate
disclosure, binding/grain/time/filter/Join validation, immutable plan/refusal writes and stable
reason codes. Model output is an input selector only.

Proof: deterministic digest golden cases, ambiguity and no-match cases, join/grain/binding blockers,
authorization non-disclosure, restart repeatability and 10,000-asset p95 benchmark.

Packet: covered by the Alpha T005 packet.

### M3-T003 REST consumers, bindings and TypeScript client

Status: Needs_Review
Delivery profile: Alpha 0.1.0
Parent execution: Alpha T005
Depends on: M3-T002 evidence complete
Blocks: M3-T004, M3-T005

Expose direct resolve, query/plan reads, consumer lifecycle and current/pinned binding lifecycle
through versioned REST. Generate the TypeScript client and emit attributable resolution/refusal
usage and audit facts without raw prompt or fact-row retention.

Proof: HTTP contract, capability denial, cross-workspace isolation, current/pinned behavior across
publish and rollback, event redaction and SDK contract tests.

Packet: covered by the Alpha T005 packet.

### M3-T004 Ask delivery

Status: Needs_Review
Delivery profile: Alpha 0.1.0
Parent execution: Alpha T006
Depends on: M3-T003 evidence complete; M2 model provider available
Blocks: M3-T006

Interpret natural language into the canonical SemanticQuery schema through the configured model,
then call the same resolver. Deliver released definition summaries, plan explanations, evidence,
clarification and refusal in Web Ask. Data intents remain visibly unexecuted when no adapter exists.

Proof: live provider evidence, schema/provider failure, prompt redaction, no fixture fallback and
desktop browser acceptance at 1440x900 and 1024x768.

Packet: covered by the Alpha T006 packet.

### M3-T005 MCP and CLI channel parity

Status: Deferred
Delivery profile: Full M3
Depends on: M3-T003 Accepted; separately confirmed MCP/CLI contract
Blocks: M3-T006

Expose released resources, direct resolution, plan/refusal inspection and binding context through the
official MCP Go SDK and Semlia CLI. Both channels call the canonical distribution application
service and pass the same authorization, digest, refusal and attribution conformance suite.

Proof: MCP resource/tool and CLI golden conformance against REST, compatibility tests and external
Agent fixture.

Packet: not authorized by the Alpha plan.

### M3-T006 Ecosystem distribution acceptance

Status: Deferred
Delivery profile: Full M3
Depends on: M3-T004 Accepted, M3-T005 Accepted; webhook and optional execution contracts separately confirmed
Blocks: M3 milestone acceptance

Add confirmed webhook/feedback delivery and any selected execution adapter, then prove that Fluxale
and one independent Agent consume released semantics exclusively through Semlia. Absence of an
execution adapter cannot block resolution acceptance; configured adapters must pass capability,
security-context, validation and provenance contracts.

Proof: external production-style journeys, webhook idempotency, channel compatibility, failure and
refusal attribution, security/release gates and independent review.

Packet: not authorized by the Alpha plan.

## Alpha Crosswalk

| M3 task | Alpha task | Status effect |
| --- | --- | --- |
| M3-T001 through M3-T003 | Alpha T005 | May reach `Needs_Review` with Alpha T005 evidence |
| M3-T004 | Alpha T006 | May reach `Needs_Review` with Alpha T006 evidence |
| M3-T005 and M3-T006 | None | Remain Deferred after Alpha acceptance |

Full M3 cannot become `Accepted` from Alpha evidence alone.
