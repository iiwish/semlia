# V1-T003 Natural Correction Evidence

## Contract

Natural knowledge correction creates one new production update draft through the existing production API. It does not call legacy proposal creation, supersede a released create operation, confirm business rules, submit validation, approve, or publish. The action is labeled `保存修订并继续确认`; success enters the existing work-operation route. The reason and answer pin appear as read-only, explicitly unconfirmed context. Authors must perform normal confirmation and submission, with independent review and publication afterward.

The editor captures its opening revision ID and content. Saving re-reads the canonical current asset, requires that exact baseline to be the published revision, and checks every submitted before value. An intervening revision or a different unpublished current draft is rejected visibly rather than silently rebased.

Source provenance starts at the published-state release and follows immutable `beforeHead` links, with cycle detection and a maximum of 64 release reads. Only an applied non-rollback release with the exact manifest pin and an exact-version production target can identify the producer. Operation identity/version/set digest, target content digest and attributed proposal membership must agree. An unrelated release or mixed operation with a `no_change` target is skipped; a purported producer with mismatched digest or attribution is rejected.

The draft preserves original verified source snapshots, coverage pins, input evidence and target evidence IDs. Source snapshot IDs/digests and complete coverage are checked through normal authorized reads. Consumed candidate links are empty. Original dependencies and typed knowledge references retain exact version pins; conflicting pins are rejected.

One command owns a frozen body and idempotency key. Concurrent clicks share its in-flight promise. Uncertain delivery locks editing and new commands; retries reuse that exact body/key without re-preparing from newer state. Once uncertain, a later permission denial does not prove that the original save failed and therefore does not unlock the command. There is no automatic retry. Identity/workspace aborts prevent late results from navigating the replacement session. No new private draft persistence is introduced.

## RED / GREEN

All tests use synthetic component/API fixtures. A003 made no database writes, model calls or V1 runtime lifecycle changes.

- The helper's first RED could not import the absent module. Its initial 14 scenarios passed after implementation.
- The workbench's first RED failed all three cases because the draft-save action did not exist. GREEN verifies the captured baseline, one in-flight save, uncertain-body lock/retry, and editable unsent content after a deterministic failure.
- Independent review supplied a mixed-operation counterexample. The focused RED rejected a `no_change` target as a mismatched producer. GREEN continues to the actual producer while keeping genuine digest/attribution failures fail-closed.
- Independent review supplied an uncertain-save authorization counterexample. The focused RED returned a normal permission error on unknown delivery followed by a 403 retry. GREEN retains uncertainty and proves a subsequent successful explicit retry uses the same body/key, with only one provenance preparation.
- Additional helper regressions cover stale current baseline, a separate unpublished draft, incorrect before value, wrong asset/revision/operation identity, unavailable or incomplete source snapshots, cycles, the 64-read bound, conflicting dependencies, immutable input preservation, and abort during preparation before any create.
- Product integration verifies the normal asset revision path creates a production command, passes its captured baseline and AbortSignal, navigates to the returned operation, retains unconfirmed context, and ignores late success after a principal change. The production panel test proves that context neither checks the business confirmation nor sends any write.

## Commands

- `pnpm --dir web exec vitest run src/knowledgeProductionRevision.test.tsx src/KnowledgeRevisionWorkbench.test.tsx src/ProductApp.test.tsx src/SemanticProductionPanel.test.tsx --maxWorkers=1`: exit 0, 4 files / 86 tests passed, 7.59 seconds.
- `pnpm --dir web typecheck`: exit 0.
- `pnpm --dir web lint`: exit 0; the same four existing Fast Refresh/effect-cleanup warnings recorded in `frontend.md`.
- `git diff --check`: exit 0. Final integration gates are recorded in the task's consolidated review evidence.

## Acceptance Boundary

Component tests do not establish successful real-runtime confirmation, review, publication or rollback. A002 owns the normal-password browser journey and the V1 runtime; its explicit run is required after independent review. The existing production harness results in `frontend.md` precede this natural-correction integration and are not claimed as proof of this new path.

Uncertain command state is memory-only. Refresh or identity change clears the local private editing session; the server's idempotency and persisted operation records remain the authoritative record. An absent or unauthorized historical production/source record blocks revision creation rather than replacing its provenance with current data.
