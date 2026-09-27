# Embedded Candidate Deep-Link and Driver Repair

## Status

V1-T004-A004 repairs defects exposed by the first direct embedded-candidate browser preflight. Backend and driver focused RED/GREEN checks and independent reviews pass. A new frozen-source candidate and full T005 gates are required before repeating candidate browser acceptance. Root selects `1.0.0-rc.20260927.2` for that separately authorized third gate attempt; its build and acceptance are not recorded as completed here. This record does not certify the failed candidate or final-model cohort.

The existing `1.0.0-rc.20260927` candidate, manifest and original browser artifacts are preserved. No model call, live browser rerun, database change or runtime restart was performed for the backend repair.

## Observed Candidate Failure

The protected run `browser/1790443392330` targets the direct embedded application at `http://127.0.0.1:52688/`, not Vite. Its four Playwright cases report **two passed and two failed** in 45.1 seconds. Both consumer keyboard/reduced-motion cases pass with zero recorded errors or denied API responses. Both author cases complete normal-password login and source inspection, then fail on the asset detail deep link before opening the knowledge revision workbench.

The failing request is `/assets/ast_01m3ev2qq8fvxtz73paszba169`. Both failure snapshots contain `404 page not found`; the subsequent locator waits for the missing `定义` tab and times out at `web/e2e-v1/preflight.spec.ts:9`. The embedded HTTP handler treats every missing path under `assets/` as a missing static resource, including supported semantic-asset routes. This is a product deep-link/refresh defect, not a flaky locator or permission denial. Navigation through the sidebar or a Vite server is not a substitute for fixing it.

The launcher independently exits 1 at `after-runtime` with `UNCLASSIFIED`. It preserves `runtime-before.json` but does not reach its runtime-after, counter or head receipt. Old source-mode preflight receipts are not evidence for this candidate run. The specific launcher exception cannot be reconstructed from that generic result alone; it must not be relabelled as a confirmed socket or login-budget failure without reproduction.

Root's separate protected `candidate-preflight-failed-independent-postflight.json` confirms the failed candidate remains ready at the same direct API address, with 17 model steps, 17 agent runs, 19 executions and zero unfinished work. The correct head is `rls_01m3f2pbg0f1br2ba4hbtgjgrs`, restoring revision `rev_01m3ev2rhdfw2vcm1atafxx7vd`. This later check supplements preservation evidence; it does not turn the failed launcher or child cases into passes.

Read-only file hashing independently matches the active and staged binary, manifest and archive to the [second frozen release-gate attempt](../V1-T005/release-gates.md):

- Binary: `sha256:b960b872ea904354165a27e4f2f189df2665afda7a3ca77d8259e49ac93e106f`.
- Manifest: `sha256:0202a3b841d2a1bb4849ebf106f2fc41129e1bd8a037c4f7220951953895ed9d`.
- Frozen source: `sha256:aa52cc10448f82ee88e8ff5e58d4dc4ed7e0ab4bde6e71c07af2342c0801abd4`.
- Root's runtime-version receipt reports `1.0.0-rc.20260927`, schema 33, and no `SEMLIA_BUILD_VERSION` override. Candidate mode does not run Vite.

## Backend Repair

Only `internal/platform/web/handler.go` and its test change. Existing embedded files retain priority. Among missing paths under `assets/`, application fallback is limited to the existing `/assets/{assetId}` and `/assets/{assetId}/versions` route shapes. The shared `identity.ParseAssetID` parser enforces the `ast` type and valid UUIDv7 identity; a prefix check alone is insufficient. Classification uses the original URL path, not a traversal-normalized surrogate. Query pins are neither interpreted nor rewritten by the HTML handler; catalog authorization and immutable-revision checks remain API responsibilities.

Missing scripts, styles and fonts, malformed or wrong-type IDs, extension-bearing names, unknown children and traversal-shaped paths remain 404. Existing static files keep immutable caching and `nosniff`; application HTML keeps `no-store` and `nosniff`. GET and HEAD work for valid deep links, with no HEAD body. Other methods remain 405 with `Allow: GET, HEAD`. API and health paths still delegate their original request and preserve 401, 403 and 503 responses without SPA fallback.

### RED

```bash
go test -count=1 ./internal/platform/web \
  -run '^TestHandlerServesAssetClientDeepLinks$'
```

Exit 1. All eight new subcases fail with actual HTTP 404 instead of 200: GET and HEAD for detail, versions, detail with revision/release pins, and versions with those pins. The test was added before the handler repair; the failure is at the status assertion, not test setup.

### GREEN

```bash
go test -count=1 ./internal/platform/web
git diff --check -- internal/platform/web/handler.go internal/platform/web/handler_test.go
```

Both exit 0; the focused web package takes 0.737 seconds. The full web suite includes the same eight positive subcases plus missing-static, typed-ID, route-shape, static-file precedence, cache/header, method and API/health boundary regressions. No assertion skips the original failing deep-link route.

An uncached adjacent-boundary run, `go test -count=1 ./internal/platform/web ./internal/platform/http ./pkg/identity`, also passes all three packages in 0.445, 1.549 and 0.479 seconds respectively. The independent reviewer reports no blocking findings and independently runs the web package with `GOENV=off`, `GOWORK=off`, empty `GOFLAGS` and `-count=1`: exit 0 in 0.422 seconds. These are handler/component checks, not a new deployed-candidate acceptance run.

## Driver Repair

The separately owned driver repair changes only the five authorized files: `scripts/acceptance/v1-browser.mjs`, `v1-browser-core.mjs`, `v1-browser.test.mjs`, `v1/runtime.mjs` and `v1.test.mjs`. Its author reports the following real local-subprocess reproduction and tests. No test connects to the retained V1 runtime, model provider or database.

### Transport Reproduction

```bash
node --test --test-name-pattern='runtime transport survives' \
  scripts/acceptance/v1.test.mjs
```

The RED test uses a separate child HTTP server with Unix control and TCP readiness endpoints, a two-second keepalive and zero keepalive timeout buffer. It performs actual `request` and `fetch` probes, blocks the parent for 2.5 seconds in a real synchronous Node child, then probes again. The control request fails with `Owned runtime control unavailable`, with an `EPIPE` cause. Exit 1, 2.660 seconds.

The repair uses one-shot control requests (`agent: false`), readiness requests with `Connection: close`, and consumed readiness response bodies. The same test passes in 2.633 seconds without retries. A separate actual-driver scenario crosses the same real service/child-process boundary and observes exactly two control and two HTTP requests, all closed rather than retained: passed in 2.671 seconds.

This reproduces a concrete stale-pooling hazard in the launcher lifecycle. It does **not** establish that the original candidate run had `EPIPE`: that run discarded its exception cause, and its specific transport failure remains unclassified.

### Durable Failure Evidence

The driver writes protected, run-specific runtime identity and counters before starting Playwright, then records the child status/signal/spawn outcome before postflight. A failed postflight retains these records and private error diagnostics; public output exposes only allowlisted stage/code metadata. Evidence writes require unchanged owned directory identities and private permissions. Failed children, signals, timeouts, spawn errors, identity changes and postflight failures remain nonzero. No retry was added.

Two actual-driver persistence tests first failed because the before-counter file was missing, including a success-path fixture that asserts evidence exists before the child starts. Their GREEN runs verify before/child evidence survives postflight failure, the exact private cause is retained without leaking a synthetic secret, and normal success records its postflight. The driver tests also exercise nonzero child exit, signal, timeout and missing executable failures. An intermediate fixture had an incorrectly escaped nested JavaScript newline and exited 13; it was corrected as test construction, not counted as a product RED or a successful browser attempt.

### Focused Validation

```bash
node --test scripts/acceptance/v1*.test.mjs
node --check scripts/acceptance/v1-browser.mjs
node --check scripts/acceptance/v1/runtime.mjs
git diff --check
```

The author's final run reports 39 tests passed, zero failed or skipped, in 3.169 seconds; both syntax checks and the diff check exit 0. The independent reviewer also reports 39/39 passing tests, zero skips, in 3.177 seconds, a clean diff check and no blocking findings, as confirmed by root. Final embedded-browser acceptance requires a fresh candidate and explicit root authorization; these local lifecycle tests are not a substitute for it.
