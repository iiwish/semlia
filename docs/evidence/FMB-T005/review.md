# T005 Delivery Review

Disposition: Needs_Review. No confirmed open P1/P2 implementation blocker remains in the reviewed scope. Full-Menu Beta acceptance is not recommended by this record.

## Specification

The implementation supplies the packet's durable released-corpus generation, exact configuration identity, leased checkpoints, cancellation, atomic activation, lexical fallback, authorized candidate recall and real model-settings surface. Migration 19 supports metadata-only PostgreSQL and explicit administrator pgvector enablement. No request performs DDL, and the existing PostgreSQL 18 development volume is retained.

The corpus is bounded to 5,000 released summaries. This is not unrestricted document ingestion or an ANN implementation. The deterministic resolver remains unchanged.

## Confirmed Findings

- Provider JSON-null components and a second credential lookup could violate vector/credential pinning. Both are corrected with RED/GREEN assertions.
- Search could compute a vector from one active generation and query another. The exact active generation and released membership are fenced; generation/release changes use lexical fallback.
- Workspace/job locking could deadlock through runtime-event foreign keys. Embedding uses `FOR NO KEY UPDATE`; deterministic job/FK barriers pass.
- Fixed-interval request epochs could starve slow polling. Single-flight requests and scoped cancellation replace that behavior.
- The rebuild dialog lacked keyboard containment and focus restoration. Tab/Shift+Tab, Escape and return focus are covered by focused tests and real browser verification.
- Same-workspace principal/authorization changes could retain old results. Settings remount on workspace, principal and authorization version; failed reads clear search results.
- The single-flight change could block initial StrictMode loading after an abort. Token-owned cleanup and asynchronous initialization fix it; independent follow-up review closes the finding.
- The model dimension form's minimum of 128 prevented editing supported two-dimensional models. The range is corrected to 1 through 4,096 with RED/GREEN form-validity tests for 2, 4,096 and 4,097. The real QA UI saves an edited two-dimensional model successfully.

## QA Evidence

- Full source gate passes with PostgreSQL 17/18 migration tests, 170 Web tests, generated drift and binary build after the final dimension-form correction.
- Real pgvector integration covers current-revision recall, authorization filtering/version changes, checkpoint reuse, lease loss, failed/cancelled retention, late enablement and rollback refusal.
- The final local deterministic live browser suite passes both desktops (2 tests, 13.1 s), including exact revision output, reload, Operations deep link, keyboard behavior, overflow and serious/critical accessibility checks.
- The complete fixture browser suite passes 26 tests in 31.9 s. These are fixture/transport-mock journeys, not hosted or external integrations.
- The repository security scanner passes. The scanned local Dockerfile explicitly enables local-UAT controls; this does not establish production authentication or hosted security acceptance.
- The latest Docker development stack is clean at migration 19. The real source/ingestion/schedule desktop suite passes 4 tests in 10.7 s; live smoke passes in 18.714 s.

## Acceptance Gaps

Live external provider execution, semantic recall-quality evaluation, 5,000-asset load/throughput evaluation, hosted TLS/OIDC and hosted release acceptance remain unverified. Recovery evidence distinguishes persistent repository checkpoint reuse from the separate graceful queued-run/server/database/worker restart; neither is claimed as an in-flight SIGKILL test.

Two ref-cleanup lint warnings and the existing KnowledgeViews fast-refresh warning are non-blocking recorded warnings. No migration history, user data, security rule or failing assertion is removed to obtain passing gates.

## Snapshot

`diff.patch` is a 103-file, 4,449,307-byte cumulative HEAD-relative source snapshot under the T005 allowed paths. It includes pre-existing Alpha/FMB work and is not a claim that all those changes were authored during T005. Its SHA-256 is `26a4c1fe9499dab1c88d54810a6d3b93bdcf903ff601e8ddba0d3ec8c15fb246`. T004's frozen snapshot is retained unchanged. The embedding data-model document describes the implemented JSON configuration, ordinal keys and transactional validation contract.
