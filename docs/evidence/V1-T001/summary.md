# V1-T001 Isolated Acceptance Lane

Status: Needs_Review. Date: 2026-09-26. Packet: V1-T001-A001.

## Verified Baseline

- Normal server, worker and Vite run on dedicated loopback ports. Web: `http://127.0.0.1:52689/`; API: `http://127.0.0.1:52688/`. Readiness succeeds after a complete owned stop/start.
- The existing repository-owned PostgreSQL container supplies two new databases and two dedicated non-administrator roles. No additional database container is installed.
- The fixed synthetic source contains seven orders and four customers. Normal source connection testing and live discovery produce two datasets and eight fields; the dedicated reader passes the normal read-only role assertions. `SEMLIA_DISCOVERY_ALLOW_UNSAFE_SOURCE=false`.
- Six distinct actors use normal password login and cookie/CSRF sessions. The author has semantic steward, source operator and reviewer roles; the approving reviewer and publisher remain separate principals.
- All ten semantic assets across five knowledge types, two physical bindings and one join contract are submitted, validated, independently reviewed and published through normal production HTTP operations. There are exactly 13 operations and 13 releases. The latest manifest contains ten assets and three governed objects.
- A self-review probe against the author's existing operation confirms the actor has `proposal.review` and receives HTTP 403 with the stable `SOD_CONFLICT` code. The probe creates no additional operation or release.
- Independent PostgreSQL queries reconcile total, regional, monthly and old-customer ratio results. Golden answers remain verifier-side; the analysis model definition has no numeric expected-answer disclosure.
- The configured enabled default LLM is copied from `semantic-core`: `deepseek-flash`, `openai_compatible`. Its source workspace and configuration fingerprint are recorded privately. Endpoint, protocol, enabled status, model kind, model name and token limit are checked on repeated initialization. T001 makes zero model calls.
- Repeated initialization leaves the same 13 releases, 13 operations and published target identities. Existing application database counts and model configuration fingerprints, native environment files and Compose overlay hashes match the preserved pre-initialization snapshot.

## Implementation

- `cmd/v1-acceptance/`: emits synthetic fixture templates without accessing application storage; Go tests cover fixed data, five types, serialization and answer-free model definition.
- `scripts/acceptance/v1/core.mjs`: private state ownership, atomic checkpoints, strict runtime environment allowlist, full fixture fingerprint, safe failure formatting and independent golden queries.
- `scripts/acceptance/v1/provision.mjs`: owned PostgreSQL inventory verification, dedicated role/database provisioning, read-only ownership revalidation, model metadata selection and synthetic source reconciliation.
- `scripts/acceptance/v1/runtime.mjs`: normal binary build, migrations, password bootstrap and an owned supervisor for server/worker/web. Database ownership is checked before migration/bootstrap and on startup/status. Stopping owned services uses protected owner/token process control and does not require an available database.
- `scripts/acceptance/v1/initialize.mjs`: normal authentication, actors, source discovery, configured model copy and governed publication.
- `scripts/acceptance/v1.mjs` and `scripts/acceptance/v1.test.mjs`: runnable lifecycle and nine focused regressions.

## Review Corrections

1. Runtime recovery revalidates both database owners/markers and both roles' markers, privilege flags and memberships before migration/bootstrap. Missing or drifted resources are rejected; `up` does not create or repair them. Pure regression cases reject changed owners, markers, elevated privileges and missing records.
2. CLI failures use a fixed diagnostic instead of arbitrary exception messages. A child-process regression reads malformed private JSON containing a fake credential and confirms only the fixed diagnostic reaches stderr.
3. Self-review evidence requires both the review capability and `SOD_CONFLICT`, not an arbitrary permission-denied response. The real probe passes while retaining the 13-operation baseline.
4. Full fixture fingerprints include synthetic data, knowledge definitions/specifications and reference structure, with random generated identities canonicalized. Same SQL with changed knowledge is rejected; fresh random identities are accepted. The owned legacy receipt retains its original database marker identity. Its full-fingerprint upgrade is recorded only after saved/current complete fixture equality and live resource ownership checks. The private upgrade receipt records `savedMatchesCurrent=true` and its verification timestamp. A missing saved fixture is never adopted for a provisioned run.

## Actual Commands

All commands run from the repository root.

```sh
node --test scripts/acceptance/v1.test.mjs
go test -count=1 ./cmd/v1-acceptance
SEMLIA_V1_MODEL_WORKSPACE=semantic-core node scripts/acceptance/v1.mjs init
node scripts/acceptance/v1.mjs init
node scripts/acceptance/v1.mjs down
node scripts/acceptance/v1.mjs up
node scripts/acceptance/v1.mjs init
node scripts/acceptance/v1.mjs verify
```

RED evidence: initial Node tests failed for missing acceptance modules; the fixture serialization regression failed with a zero workspace identity; the execution source configuration regression failed for an invalid environment-variable prefix. The review-correction tests were added before their missing exported implementations and failed module instantiation. GREEN evidence: all nine Node tests pass; the Go fixture package passes; real initialization, reinitialization, stop/start and final verification succeed. Initial configuration ambiguity stopped before creating databases and was resolved by the orchestrator's explicit `semantic-core` selection. Initial execution startup rejected an unsupported DSN variable prefix; the prefix is covered by the passing regression. No application production contract was weakened.

Existing full-suite and browser baseline evidence is recorded separately in `baseline-tests.md` and `browser-baseline.md`; this task does not claim that the old browser tests or the full V1 scope pass.

## Protected Resources

Owner: `semlia_v1_ca0d61461449b91b`. Databases: `semlia_v1_ca0d61461449b91b_app` and `semlia_v1_ca0d61461449b91b_source`. Roles: the owner and `semlia_v1_ca0d61461449b91b_reader`.

Private root: `.semlia/v1-acceptance/current/` relative to the repository (0700). Private JSON receipts, session cookies and logs are 0600. `state.json` records resource, publication, discovery, self-review and fixture fingerprints; `secrets.json` holds credentials and source model metadata; `golden.json` holds verifier SQL/results; `baseline-report.json` records this baseline; `sessions/` holds normal login cookies. None belong in committed evidence. Account names use `v1_admin`, `v1_author`, `v1_reviewer`, `v1_publisher`, `v1_consumer` and `v1_denied`; credentials must be consumed internally, never printed.

`down` stops only the owned supervisor's children and retains every database and receipt. It does not prune Docker resources or delete data. Unknown ownership, occupied ports and conflicting runtime receipts are refused. An abruptly killed `init` can leave `operation.lock`; there is intentionally no automatic lock takeover. Recovery requires checking that no initialization process remains, validating the protected owner/state and lock path without symlinks, and explicitly removing only that stale lock before repeating `init`. Never delete the state, fixture, secrets, socket or database to bypass a failed ownership check.

## Remaining Scope

- Real model Ask/Execute, SQL/result equality against all four goldens, correction, denial/timeout/cancel/idempotency, distribution adapters and browser journey are T002 and later. A configured model is not evidence of successful inference.
- PostgreSQL and HTTP use plaintext only over local loopback for development. This is a transport exception, not a read-only permission bypass and not a production deployment claim.
- No existing demo seed, destructive governance integration reset, remote deployment, shared model change or real business data was used.
- The lane remains running for the next acceptance task. Do not run concurrent initialization or lifecycle commands; controlled stale-lock recovery is manual.
