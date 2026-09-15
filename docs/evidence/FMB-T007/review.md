# FMB-T007 Review

Status: Incomplete implementation; review does not pass. No acceptance recommendation.

## Open Implementation Findings

| Priority | Finding | Required closure |
| --- | --- | --- |
| P1 | Distribution canonical JSON decodes numbers through float64, while the execution compiler preserves json.Number. Adjacent integers above 2^53 can share a plan/request digest but bind different values. | Preserve numeric precision and prove distinct large-integer/decimal digests and arguments. |
| P1 | Execution projection uses free-form content field pairs rather than the frozen JoinContract's formal left/right references and expression contract. A composite tenant key can be omitted. | Authoritative frozen field-pair projection, closed-form expression validation or refusal, compound-key negative and positive tests. |
| P1 | Normal publication records one changed asset or governed object per release, while distribution requires the complete executable dependencies in that release. Hand-built complete-release fixtures conceal this unreachable path. | A real proposal/review/publish-to-resolve-to-execute database journey with immutable, already-published dependency composition. No mutable draft or historical physical backfill. |

These findings are from independent read-only inspection of landed compiler, projection
and publication code during implementation. They are assigned to the sole worker;
unfinished service/adapter files are not treated as final implementation.

## Release Findings

- Default Docker builds enable local-UAT identities; production defaults must exclude
  them while local Compose explicitly opts in.
- Existing release provenance names HEAD despite a dirty worktree and has no actual
  migration/source fingerprint. Candidate evidence must bind the tested dirty source.
- Existing SBOM generation scans the staged Go executable but does not establish
  coverage of pnpm-locked embedded-Web dependencies. A complete SBOM claim requires
  verification of both dependency sets.

## Runtime Boundary

Preflight browser inspection confirms local UAT is not a real OIDC session. The member
directory in that mode is explicitly demonstration data; production SessionRuntime
selects the server-owned member view. Hosted and local evidence remain separate.

## Frozen Handoff Assessment

The worker stopped around 08:10 UTC after a quota error and resumed at 10:30 UTC only
for handoff and bounded validation cleanup. The execution core is not connected to
HTTP/MCP/CLI/SDK/Ask or the application composition root. Publication composition is
not implemented. It is not an executable product feature.

The digest helper uses `Decoder.UseNumber` in the frozen source, but the requested
adjacent-large-integer/high-precision regression cases are absent. The join compiler
requires `field_pairs_equal/v1`; the loader does not populate this marker, so joins
fail closed rather than proving correct formal-contract execution. Both need further
implementation and targeted evidence; these findings are not fully closed.

Root's final source gate passes formatting, lint and typecheck but fails tests. Six
migration tests still assume schema20 or relative rollback offsets based on schema20.
The tests observe clean schema21, which proves application of the migration on those
fresh test databases, not its populated lifecycle or production readiness. Readiness
still requires20, so the preview must not be upgraded as a candidate.

An independent Operations rerun reproduced a separate test-clock failure: its job
lease expired at fixed 10:01 UTC, before the current wall clock. The bounded fixture
fix uses a real future lease deadline, preserving fixed business-event times and the
production fence. This is test cleanup, not execution-service validation.

Web regression passes 180/180; SDK typecheck and contract/sqlc drift checks pass.
Execution application and adapter packages have no tests. Source, security, release,
full execution migration and browser acceptance are not claimed as passing.
