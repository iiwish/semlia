# FMB-T005 Delivery Evidence

Status: Needs_Review. Local implementation and review gates pass; release acceptance is not claimed.

## Delivered Scope

- Durable generation metadata, immutable-at-creation released corpus snapshots, persisted vector checkpoints, per-workspace building/active uniqueness, and transactional active replacement.
- Exact provider, endpoint, model, dimension, credential revision and chunking identity. Vector reuse requires the same configuration digest and chunk digest in the same workspace.
- Bounded OpenAI-compatible HTTP batches with index/cardinality/dimension/nonzero/finite validation, explicit JSON-null rejection, one pinned credential lookup, redirect rejection, response limits and timeouts.
- Worker lease and cancellation fences; failed or cancelled generations retain the existing active index. Operations records use the real run and job identities.
- Vector search uses the exact captured active generation and current published release. Generation/release changes fall back to lexical retrieval. Every returned asset is filtered using current authorization and an authorization-version fence.
- Migration 19 permits metadata deployment without pgvector. The administrator-only enable script installs the extension/vector column; runtime requests never perform DDL. Capability detection checks the current schema and actual vector column type.
- Model settings show server-backed status, checkpoints, rebuild confirmation, cancellation, exact Operations links and released-knowledge search. The browser-only 42-percent rebuild state and fabricated callbacks are removed.
- Workspace/principal/authorization-version changes remount settings. Slow polling is single-flight, StrictMode aborts do not block initialization, and confirmation supports Tab containment, Escape and focus restoration.

## Files

- Domain/application: `internal/domain/embedding/{model.go,model_test.go}`, `internal/application/embedding/service.go`.
- Provider: `internal/adapters/embedding/{openai.go,openai_test.go}`.
- Persistence: migration 19 up/down, `db/queries/embedding.sql`, `db/sqlc.yaml`, generated sqlc, `internal/adapters/postgres/{embedding.go,operations.go}`.
- API/composition: OpenAPI and generated Go/TypeScript contracts, `internal/platform/http/{embedding.go,handler.go,catalog.go}`, `cmd/semlia/{main.go,readiness.go,readiness_test.go}`.
- Web: `embedding.ts`, `embeddingRuntime.tsx`, `embeddingRuntime.test.tsx`, `ModelConfigurationView.tsx`, `ProductApp.tsx`, `ProductApp.test.tsx`, `localUAT.ts`, `localUAT.test.tsx`, `styles.css`, generated embedded assets.
- Validation/operations: `tests/integration/embedding/embedding_test.go`, migration inventory expectations in `tests/integration/db/database_test.go`, `web/e2e/product-journey.spec.ts`, explicitly enabled read-only `web/e2e-live/embedding-live.spec.ts`, `deploy/local/enable-pgvector.sql`, `docs/operations/embedding.md`, and this evidence directory.

## Boundaries

- Integration and isolated browser QA use deterministic HTTP embedding fixtures, not a live external model. No semantic-quality, hosted deployment, external-credential, throughput or ANN benchmark gate is claimed.
- Corpus scope is at most 5,000 released asset summaries, one bounded address/name/title/description chunk per asset. Raw source bytes, unpublished revisions and unrestricted document ingestion are outside this implementation.
- Changing defaults does not mutate an existing generation's configuration. Removed/rotated pinned credentials make that generation's provider calls unavailable rather than silently using another credential.
- Migration rollback refuses existing index history; it does not delete generations to permit downgrade. The administrator enable script targets the installed PostgreSQL major version and does not replace the existing development PostgreSQL 18 volume with PostgreSQL 17.
- Existing generated/shared changes from T001-T004 are preserved. No commit or push is performed.
- Final source-gate, browser, restart and independent-review results are recorded in `test-results.md`, `review.md` and `orchestrator-qa.md`. The final source gate includes 170 Web tests; real embedding desktop E2E passes both supported sizes.
