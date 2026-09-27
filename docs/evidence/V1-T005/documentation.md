# V1-T005 A001 Documentation

Status: the documentation batch and follow-up checks pass. The current [T005 delivery record](summary.md) reports RC.6 technical verification and user-delegated browser acceptance. Git delivery is authorized with PR CI as the merge gate; formal release remains separate. The attempt-specific validation and failures below are preserved, not presented as final-candidate execution.

## Scope

Only these current documentation surfaces are edited: `README.md`, `README.zh-CN.md`, `docs/README.md`, `docs/SSOT.md` current-scope/approval metadata, `docs/quickstart.md`, `docs/operations/local-development.md`, `docs/operations/troubleshooting.md`, `docs/operations/release-verification.md`, and the new `docs/operations/backup-recovery.md`. The explicitly authorized `.env.example` edit changes only its introductory comments to identify the manual `ensure-env.sh` step. No behavior, dependency, generated asset or historical evidence is changed in this batch.

The separately authorized follow-up also updates only `TestM0DocumentationContract` in `tests/acceptance/documentation_contract_test.go` to enforce the current native documentation contract, retaining its security-reporting and diagnostic requirements.

The documents state the scoped local 1.0 candidate and its pending final gates; the intended identifier is `1.0.0-rc.20260927`. They distinguish the implemented five-type knowledge, governed publication, published-model Ask/Execute and ordinary machine channels from long-term SSOT capabilities. Desktop support is 1024px or wider, not mobile. T001–T004 evidence is linked with a pending T005 evidence entry rather than an invented final receipt.

## Fact Audit

- Native defaults were checked against `Makefile`, `scripts/dev/native.mjs`, `scripts/dev/ensure-env.sh` and `scripts/dev/account.sh`: existing PostgreSQL, private configuration, explicit migration/account bootstrap, native server/worker/Vite, health-only `make smoke`, owned-process-only `make dev-down`.
- New-environment instructions specify PostgreSQL 18, dedicated database/role and hidden password entry; migration 33 and trusted `pg_trgm` are explicit. No vector extension, database replacement, unknown-listener shutdown or environment-file deletion is recommended.
- The explicit `SEMLIA_NATIVE_ENV_FILE` branch is checked against `scripts/dev/native-environment.mjs`: required independent file plus explicit `SEMLIA_NATIVE_STATE_DIR`, no fallback to normal configuration or inherited runtime credentials, no file-defined reserved controls. Default configuration merge remains distinct. [T004-A003](../V1-T004/browser-gate-isolation.md) records the implementation, independent review and one complete `make check-browser` run: exit 0 in 115.778 seconds, native password/member checks and 12 production tests, with all eight preservation/cleanup checks passing. This is source-environment evidence, not the final frozen RC gate.
- Self-host Compose is checked against `deploy/examples/compose.yaml`: existing database network, protected migration/runtime configuration and successful-migration dependencies for server and worker. Isolated root development Compose is not an existing shared-database deployment.
- Recovery requirements match T004: application DB, nonempty content/inputs/artifacts, key material, external credentials, consistent write freeze, exact ownership and empty new target, byte/row fingerprints, normal login/pins/preview and actual stored-credential decryption. External source data has a separate backup responsibility; ephemeral rows are not promised recoverable. SQL preview HTTP 422 is an adapter boundary.
- Execution prerequisites include role-scoped `default_transaction_read_only=on`, `log_min_error_statement=panic` and an explicitly limited loopback plaintext exception, not a permission bypass.
- Natural revision key lifetime and the 64-step historical-producer limit were checked against `web/src/knowledgeProductionRevision.ts`. No persistent cross-refresh recovery claim is made.
- Prior development-tool private-configuration disclosure is noted without values; affected credentials require approved rotation before production, preserving ciphertext recovery.

## Validation

`go test -count=1 ./tests/repository ./tests/contracts` returned exit 1. Contracts passed. Repository checks reported two findings outside this batch's allowed changes:

1. `TestSmokeJourneyUsesActiveComposeProject` requires old `COMPOSE_PROJECT_NAME` strings in the changed smoke helper contract.
2. `TestDocumentationContainsNoPersonalAbsolutePaths` identifies a personal absolute path in historical `docs/evidence/V1-T001/summary.md`.

Both were reported to the orchestrator; neither test nor historical evidence was weakened or modified by this batch. The interim named-skip run below is retained only as bounded documentation feedback. The owning tasks repaired the smoke contract and path portability; the full unfiltered repository/contracts rerun is recorded separately.

| Command/check | Result |
| --- | --- |
| Initial `go test -count=1 ./tests/repository ./tests/contracts` | Exit 1; the two repository findings above, contracts passed |
| `go test -count=1 ./tests/repository -skip 'TestSmokeJourneyUsesActiveComposeProject\|TestDocumentationContainsNoPersonalAbsolutePaths'` | Exit 0; explicitly limited feedback, not the full gate |
| `go test -count=1 ./tests/repository -run '^TestDocumentationContainsNoPersonalAbsolutePaths$'` | Exit 0 after the orchestrator made the T001 path-only portability correction |
| Final unfiltered `go test -count=1 ./tests/repository ./tests/contracts` after T004-A003 | Exit 0; repository 5.458 seconds, contracts 1.779 seconds; no tests skipped |
| Final Node relative-link audit of all 10 edited documentation files | 161 local links checked, zero missing targets |
| Stale-fact search across README, quickstart and runbooks | No unsafe deletion/default listener stop, schema-29 requirement, disruptive daily-smoke or M1-only current-status claim; roadmap M1 terminology intentionally retained |
| `git diff --check` | Exit 0 |

Relative-link and stale-fact audits cover only the edited documents. Final local candidate compilation, signed receipts, Docker security checks, real-model calls and environment lifecycle operations are not part of this documentation attempt.

Independent documentation feedback also identified a bare second `make check` that would not inherit a previous command's security-image override, and unquoted shell placeholders. The commands use an explicit fresh owned image variable for either the complete gate or the individual security gate; placeholder arguments are quoted and require replacement. These corrections do not change any executable behavior.

## Source-Gate Documentation Follow-Up

The first full source gate exposed the old M0 static quickstart contract and missing troubleshooting diagnostic names. A focused real RED, `go test -count=1 ./tests/acceptance -run '^TestM0DocumentationContract$'`, exited 1 with six failures: two old port-8080 URLs, the obsolete `preview` and `disruptive` terms, and absent `DEPENDENCY_UNAVAILABLE`/`traceId` guidance. This failed attempt remains part of the source-gate evidence; unrelated frontend asynchronous failures belong to their owning task.

The minimal update requires native Web readiness at 18081, direct API system info at 18080, existing PostgreSQL, explicit environment/migration/account setup, health-only daily smoke and owned-native-process shutdown. Existing private security-reporting requirements and dependency diagnostics remain enforced. Additional negative checks reject environment-file deletion, the obsolete disruptive daily-smoke instruction and stopping an unknown listener. Troubleshooting explains stable dependency codes, protected trace correlation and readiness recovery.

| Follow-up validation | Result |
| --- | --- |
| Focused `TestM0DocumentationContract` GREEN | Exit 0, 0.806 seconds |
| Unfiltered `go test -count=1 ./tests/repository ./tests/contracts` | Exit 0; 5.903 and 2.168 seconds respectively |
| Relative-link audit | 10 documents, 161 local links, zero missing targets |
| `gofmt` on the authorized test and `git diff --check` | Exit 0 |

No live environment, model, generated asset, dependency or other test was changed. These results do not turn the first complete source gate into a pass; the orchestrator must rerun the full gate on the frozen candidate inputs.

## Final RC.4 Status Batch

Current README, documentation index, SSOT scope metadata, release instructions, task status and T005 delivery records identify the verified RC.4 candidate and pending user acceptance. Historical attempts keep their original failures, cache/skip distinctions and model denominators. Private credentials remain outside the public documents.

| Final documentation validation | Result |
| --- | --- |
| Unfiltered `go test -count=1 ./tests/repository ./tests/contracts` | Exit 0; 6.064 and 1.930 seconds |
| Focused `TestM0DocumentationContract` | Exit 0, 0.500 seconds |
| Inline relative-link filesystem audit | 15 documents, 203 targets, zero missing; remote URLs and anchors are not validated by this check |
| Official release source fingerprint | Unchanged `sha256:a413933ffe5c282a1ea845a31ff730070271b251baeeb1f245c9f01cef92ad86` |
| `git diff --check` | Exit 0 |

These are documentation closeout checks, not a repeat of the real-model cohort or a substitute for the independently recorded complete sixth gate attempt.

## RC.6 Delegated Acceptance Closeout

Current README, documentation index, SSOT scope metadata, release instructions, tasks and T005 reports identify RC.6 delegated acceptance, conditional Git authorization and the separate formal-release boundary. They distinguish fresh frontend/browser/smoke/security/release checks from inherited Round 7 Go/Node/performance evidence. Original failures and per-candidate denominators remain intact.

| Check | Actual result |
| --- | --- |
| Unfiltered repository/contracts tests | Exit 0; 5.214 / 1.354 seconds |
| M0 documentation contract | Exit 0; 0.515 seconds |
| Inline local-link audit | 46 changed/new Markdown files, 220 paths, no missing targets; remote URLs and anchors excluded |
| Official source fingerprint | Unchanged `sha256:8bdaf67c031c83f8adf9fa6ce3a4e1c0f6d9b491315f3c41dab62b9b4a72e263` |
| Diff whitespace validation | Exit 0 |

This documentation validation does not claim PR CI or merge before those actions occur; Git results are recorded by the actual PR and private handoff receipts. Private credentials, runtime data, full model responses and screenshots remain ignored.
