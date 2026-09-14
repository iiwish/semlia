# FMB-T007 A002 Review

Status: In progress. No final signoff.

The A001 findings and required proof remain in `../../review.md` and the current packet.
One worker implements; the orchestrator and a read-only reviewer independently check
source and evidence. A001's frozen source patch and logs are preserved.

## Open Early Findings

| Priority | Boundary | Required closure |
| --- | --- | --- |
| P1 | DSN primary locator validation permits TLS multi-host fallbacks to another source and prefer-mode downgrade. | Validate or reject all fallbacks, require exact authority and no implicit transport downgrade; test both cases. |
| P1 | DSN `default_query_exec_mode=simple_protocol` survives RuntimeParams reset and interpolates sensitive values into SQL. | Explicitly force an extended-protocol mode and consistent cache settings; prove DSN overrides cannot enable literal interpolation. |
| P2 | Durable deadline starts before potentially blocked claim/reauthorization, while adapter starts a fresh timeout afterward. A still-running query can outlive its claim and release its concurrency slot. | One bounded absolute execution lifetime covering claim, reauthorization and adapter; delayed-claim/validation tests. |
| P2 | Common precision/length-qualified database types are refused while invalid bigint/date/UUID parameters pass compilation. | Bounded typmod parsing, pre-connection value validation and real precision/length column tests. |
| P1 | Rollback's latest-release check is outside the workspace-locked transaction; a concurrent later publication can be overwritten by the full old manifest. | Recheck latest sequence after workspace lock before restore; race regression. |
| P2 | Migration21 down removes physical pins/history without restoring original snapshot/status contracts, making re-upgrade lose executable provenance. | Reject populated destructive downgrade and preserve all facts; restore original contracts for a permitted empty downgrade, with lifecycle tests. |

Source-library inspection confirms these are actual pgx configuration/lifecycle paths,
not hypothetical unsupported inputs. The implementation worker has each finding.
No database or Docker commands are run by the reviewer.

Root also requires an explicit source-database statement/error logging policy before
claiming no compiled SQL or values are retained. Application redaction alone does not
control source-server logging.

## Published Provenance Checkpoint

Actual proposal/review/publication-to-resolve/read-only-query/replay passes in the
worker's isolated PostgreSQL journey (`real-execution-pass.log`). It does not prove
channel or browser integration yet.

Root identified dataset-only physical snapshot aliasing: one binding could inherit
another binding's newer projection or grant a legacy binding missing physical proof.
The implementation introduces per-binding/version immutable pins and rejects differing
physical payloads for the same dataset. Independent source review confirms legacy
missing pins do not borrow a sibling binding's projection; focused negative tests and
final database gates remain required.

Independent source review also confirms carry queries select published pins and object
snapshot inheritance uses the original immutable version, not a mutable draft. The
concurrent rollback and downgrade issues above remain separate required closures.

## Cancellation And UI Checkpoint

The second read-only review identifies two additional P2 findings:

- Cancellation can race with completion: the repository ignores a zero-row update,
  and the service returns a locally constructed `running/cancelRequested=true` response
  even when the persisted run has completed. Require an authoritative returned state
  and a controlled completion-between-read-and-update regression.
- Cancellation reuses new-execution freshness checks. A newer release or physical
  revision can prevent an otherwise authorized owner from cancelling an active old
  run. Preserve current identity/credential/resource/ownership checks without requiring
  the old run to remain eligible for a new execution. Execute/replay freshness remains
  mandatory.

Root also identifies stale Ask results and idempotency keys across workspace/plan
changes. Worker reports focused UI regressions covering slow prior-workspace responses,
metadata reload and cancellation; final source and desktop verification remain open.

`actual-process-loss-rerun.log` passes a real subprocess-kill regression. Source inspection
confirms the test observes a source SELECT waiting on a PostgreSQL lock, kills that
process, waits beyond the durable deadline, and checks an unknown metadata-only replay
without another adapter call. The earlier `actual-process-loss.log` publication failure
remains an explicitly reported intermittent failure pending diagnosis.

`migration21-lifecycle-pass.log` passes actual empty 21-to-20-to-21 migration and a
populated downgrade refusal. The latter preserves the immutable pin and honestly
retains the migrator dirty marker for operator repair; it is not a successful populated
downgrade. These focused passes do not replace the complete regression gate.

## Scope Accounting

The permitted source list includes the release target in `Makefile` and the focused
`internal/adapters/postgres/distribution_execution_test.go` regression file. The worker
reported both scope omissions. `Makefile` was clean at the recorded A002 preflight;
its baseline is recovered from the unchanged preflight HEAD
`source-revision-redacted`, not from the modified file. Its original
SHA-256 is `a5789bf81c6240cba6d0068c53547589736c2bfd6d66a553a8e9abbab098f318`.
The focused regression file is new in A002 and has no preflight content.

The root preflight capture has a glob-handling defect: `web/e2e*/**` was treated as a
literal prefix, omitting six existing browser-test files. Their preflight bytes cannot
be proven from that capture. They are excluded from the attempt-relative patch and
preserved as current-source supplements with explicit unavailable-before hashes in
the manifest, not falsely represented as newly created in A002. The three execution
browser harness files are new A002 files and are included in the patch. Final source
and artifact fingerprints cover the actual candidate independently of this attribution
limitation.

## Final Source Review

The last independent read-only review closes all identified source P1/P2 findings,
including the current-binding machine cancellation case. The cancellation access path
checks current binding/consumer/expiry/credential/owner and per-asset permissions without
selecting a newer release; the execution path still selects and validates the current
snapshot and digest. A new real publication-to-REST-cancellation subtest passes.

The reviewer also confirms source-level closure for precision/types, formal joins,
DSN fallbacks/protocols, absolute deadlines, immutable physical pins, locked rollback
revalidation, populated downgrade protection, production embed, GOWORK, full SBOM
dependency-set checking and tar/staging consistency. This is not a runtime signoff:
root's complete source, security, artifact and desktop gate results remain decisive.

## Final QA Finding

Root's authenticated browser suite passes five of six tests after repairing obsolete
capability and expiry fixtures. The compact-desktop member status fails Axe color
contrast: `.member-state-active`, `#228070` on `#e9f6f3`, 4.31:1 rather than 4.5:1.
This serious accessibility finding remains open, with the assertion intact. The
source review closure above is not an all-QA acceptance recommendation.

The final release-helper review confirms digest-pinned dual Trivy scans, official
CycloneDX merge and fail-closed schema validation. `COPYFILE_DISABLE=1` prevents
macOS AppleDouble entries rather than weakening strict tar-content verification.
The actual candidate gate passes. The candidate predates final test-only fixture
updates, which the source fingerprint includes; exact final-tree release alignment
requires another candidate after the outstanding UI fix.

Root inspects actual 1440x900 effective-access and 1024x768 live revoked-client
screenshots; the viewed surfaces have readable labels and no incoherent overlap.
These representative observations do not waive the separate auth contrast failure.
