# FMB-T006 Test Results

Status: Needs_Review. These are actual local results, not hosted acceptance.

## Final Gates

| Gate | Actual result | Evidence |
| --- | --- | --- |
| `GOFLAGS=-p=1 make check-source` | PASS: all eight gates; 180/180 Web tests, SDK, all Go packages, contract/sqlc/embedded drift and build | `check-source.log` |
| `go test -p 1 -count=1 -v ./tests/integration/db` | PASS, 34.972s; uncached PostgreSQL lifecycle, denial audit, scope/action negatives, concurrency, channel parity, HMAC receiver, retries, lease fencing and migrations | `database.log` |
| Real Docker distribution E2E | 2/2 PASS, 5.8s; desktop and compact desktop credential plus webhook lifecycle | `live-e2e.log` |
| Complete fixture browser regression | 26/26 PASS, 30.6s; existing role and separation-of-duties assertions retained | `fixture-e2e.log` |
| `make smoke` | PASS against the running normal preview | `smoke.log` |
| `make security-check` | PASS; configured HIGH/CRITICAL findings zero, no secret finding | `security.log` |
| Root visual/keyboard inspection | PASS: no horizontal overflow, standard checkbox geometry, modal trap/return, secret dismissal and truthful persisted state | `screenshots/`, `visual-qa.md` |

Source-gate caches are reported as such in the log. The separately retained database
run is explicitly uncached. Serious/critical Axe findings are zero on the integration
region checked by the real desktop journey. Three existing lint warnings and the
existing >500kB bundle advisory remain nonblocking; no timeout is increased to hide a
failure. No production data volume is reset or pruned.

## RED / GREEN

| Slice | Actual RED | Actual GREEN |
| --- | --- | --- |
| Machine verifier | `TestMachineTokenEntropyAndVerifier`: undefined implementation | identity application unit test and PostgreSQL package compile pass |
| Current replay authorization | `TestReplayAndKnownIDsRecheckAssetAuthorization`: replay, query and plan accepted revoked asset | distribution/authorization/identity focused packages pass |
| Independent fanout and SSRF | `TestRouterFanoutDoesNotBlockOtherSubscribers` called only one subscriber; destination test undefined | jobs router and safe webhook transport tests pass |
| MCP CLI artifact | `TestMCPCommandValidatesClientEnvironmentWithoutServerConfig` returned generic command usage | same test passes with explicit `semlia mcp` command |
| Real Integration Settings | new view tests observed no API load / no authoritative load-failure state | seven persisted-state unit tests pass |
| Readiness 20 | `TestReadinessRequiresMachineDistributionMigration`: required 19, expected 20 | focused readiness test passes |
| Credential action set | `TestCredentialActionSetIsBoundedDistinctAndKnown`: missing validator | identity package passes; accepts only one/two distinct read/resolve actions and rejects duplicates, excess entries and execute |
| Workspace grant HTTP boundary | real browser POST400; focused HTTP TypeID case400 versus expected201 while UUID case passed | handler uses the typed WorkspaceID parser to normalize public scope IDs; six HTTP cases pass including legacy UUID and foreign/malformed rejection |
| Connected Settings disclosure | focused test observed the obsolete Prototype management banner | connected/fixture disclosure tests pass; connected page has no obsolete warning and fixture page remains explicit |

The first full HTTP regression run exposed public health requests being intercepted by
bearer parsing. Public routes remain outside authenticated machine dispatch; the full
HTTP package passed after correction. Initial webhook retry proof needed its explicit
database due-time fixture moved into the past to avoid host/database clock skew.

## Focused Results

- `go test -p 1 ./internal/application/distribution/... ./internal/application/authorization/... ./internal/platform/http/... ./cmd/semlia/...`: PASS.
- `go test -p 1 ./internal/application/webhooks ./internal/application/distribution -count=1`: PASS, including structural refusal provenance / legacy fail-closed and stale lease continuation.
- `go test -p 1 ./internal/adapters/webhooks -count=1`: PASS, including IPv4/IPv6 private/reserved destinations, mixed DNS answers, revalidation on rebinding, pinned dial and redirect refusal.
- `go test -p 1 ./internal/platform/http -run 'TestMachineRate|TestCredentialRotation' -count=1`: PASS for credential/workspace quotas, bounded expiring storage, actual HTTP 429/Retry-After and unsupported grace rejection.
- Actual PostgreSQL machine test: PASS for verifier privacy, immutable server-derived context, cookie+bearer/forged context, rotation, grant revocation, suspension and expiry.
- Actual channel matrix: PASS for resolve digest, describe definitions, search, plan/query inspection and canonical refusal through REST, official MCP Go SDK client, real CLI process and Node TypeScript SDK.
- Official SDK `semlia mcp` stdio process round-trip and resource read: PASS. Hosted and stdio sessions reject the next request after credential rotation.
- Real HTTP webhook receiver: PASS for HMAC/timestamp/event identity, immutable duplicate/retry bodies, nested safe payload, independent subscriber/Git outcomes, encrypted signing material, rotation cancellation and endpoint/filter edit cancellation.
- Actual PostgreSQL dead-letter/replay: PASS for eight-attempt dead letter, bounded jitter, optimistic replay, one additional eight-attempt budget, preserved event identity and durable replay audit.
- Actual PostgreSQL lease fencing: PASS for expired A / reclaimed B / late A finish rejection with B authoritative.
- Populated migration 19 -> 20 -> 19 -> 20: PASS, retaining two seeded releases.
- `pnpm --dir sdk/typescript test`: PASS (TypeScript compiler script).
- `pnpm --dir web exec vitest run src/IntegrationSettingsView.test.tsx --maxWorkers=1`: 7/7 PASS.
- `pnpm --dir web exec vitest run src/localUAT.test.tsx src/IntegrationSettingsView.test.tsx --maxWorkers=1`: 9/9 PASS.
- `pnpm --dir web exec vitest run src/IntegrationSettingsView.test.tsx src/ProductApp.test.tsx --maxWorkers=1`: 45/45 PASS before the additional five focused UI cases.
- Lint: PASS with zero errors and three pre-existing warnings outside T006's view. The new view warning was removed.

## Broad Runs And Runtime

The first full Vitest run passed 170/172. The two failures expected simulated credentials
in ProductApp's fixture mode. Their current assertions require the explicit unavailable
state and disabled mutations. Focused rerun passed.

Root's next full Web run during Docker contention passed 175/177 across 21 files in
404.83 seconds. All seven IntegrationSettings cases passed. The two failures were an
existing ProductApp data-ingestion 15-second timeout and a LiveSources focus timing
assertion. An unchanged serial rerun passed177/177; the final expanded source suite
passes180/180. No unrelated timeout or source change hides these failures.

Root's uncontended rerun passed all 21 files / 177 tests in 25.89 seconds, recorded in
`.semlia/sprint-t006/web-tests-serial.log`, without changes to unrelated timeouts or
focus code. Subsequent bounded changes add one connected-disclosure test and actual
Webhook settings E2E. Root's 1024px inspection found inherited 34px checkbox sizing;
scoped 16px checkbox rules and real credential/webhook geometry assertions cover it.

The first full serial integration/contracts run was interrupted at root's request after
Docker container creation and root's Docker status calls stalled. Its database package
reported five stale migration expectations (latest 19 instead of 20, and migration18
setup stepping down two instead of three); test-only assertions were corrected. A later
identity, M1 and Operations containers failed to start while a Docker rebuild was
active. The final serial source gate and uncached database run pass after Docker
contention is removed. The interrupted run itself is not recorded as a product pass.
Exact selected tool-output excerpts and stop procedure are retained in
`interrupted-integration-excerpts.log`. This worker stopped only its own go-test process
group, without any Docker cleanup or unrelated process changes.

Root owns preview build/restart and desktop runtime proof. The first actual preview
run exposed readiness19 versus migrated database20; the second build exposed type
errors in newly added unit-test fixtures. Both are explicit RED evidence, followed by
passing final builds. Root's local capability RED is recorded in
`live-e2e-red-local-capabilities.log`; only local-author's server-backed administrative
capabilities are aligned, preserving reviewer/publisher separation and build gating.
Real browser REDs also discriminate grant TypeID normalization and immediate secret
dismissal during refresh. Focused tests and final real journeys close both findings.
Locator, animation measurement and asynchronous checkbox-state test corrections retain
the corresponding exact accessibility, geometry and persisted-state assertions.

No external hosted receiver, third-party MCP installation or published SDK-registry
acceptance is claimed. Local receiver and actual client entry-point proof are separate.
