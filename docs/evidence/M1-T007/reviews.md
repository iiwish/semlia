# M1 Production Review Record

## Spec Compliance

Result: Pass, with no unresolved P0/P1/P2 findings.

- The exact immutable source reference is preserved through discovery, evidence, semantic revision,
  the production HTTP handler, deterministic Git projection and privacy-bounded usage delivery.
- PostgreSQL remains authoritative; every mutable selection points to an immutable revision and the
  database rejects cross-asset usage revision attribution.
- Public identities are TypeIDs while UUIDv7 remains an internal storage representation.
- M2/M3 inference, automated review, semantic execution and remote Git workflows remain excluded.

## Maintainer Review

Result: Pass after findings were fixed and the owning tests were rerun.

| Finding | Resolution |
| --- | --- |
| JSON values above JavaScript's safe integer range could be rounded by `float64`. | Catalog canonicalization, HTTP decoding and Git rendering use `json.Decoder.UseNumber`; regression tests cover `9007199254740993`. |
| A usage row could name an asset and a revision belonging to another asset. | Migration 000004 adds a composite foreign key from `(asset_id, revision_id)` to `asset_revisions(asset_id, id)`. |
| The local Git volume was not writable by the distroless non-root worker. | The release image seeds the volume mount point with `nonroot` ownership and the full Compose journey proves worker restart and persistence. |
| Smoke fixtures still inserted legacy string IDs after the UUIDv7 migration. | Smoke fixtures generate UUIDv7 workspace and run identities and pass shutdown/restart recovery. |
| Success responses omitted the runtime trace header from the generated contract. | Every Catalog and Discovery success response declares required `X-Trace-ID`; generated clients were refreshed. |
| Current dependency scans reported fixed High/Critical advisories. | Go 1.26.6, `x/crypto` 0.55.0, `x/mod` 0.40.0, gRPC 1.83.1, `go-archive` 0.3.0 and `nanoid` 3.3.18 pass repository and image scans with zero findings. |

## QA Acceptance

Result: Pass, with no unresolved P0/P1/P2 findings.

- PostgreSQL 17 and 18 migration up/down/up, populated-M0 recovery and transactional retry pass.
- Catalog cursors return stable non-duplicated pages; exact-address search returns the golden asset.
- Projection replay is a no-op, stale bases fail without partial content and failed outbox delivery is retryable.
- Usage contains only the two allowed event types, HMAC fingerprints and bounded fields; raw queries,
  credentials, UUIDs and customer fact rows are absent from public/projection payloads.
- Source, smoke, contract drift, generated artifact, security and release gates pass.

## Accepted Boundaries

- M1 runs one worker for each configured local Git repository. Cross-process locking, remote push and
  pull-request policy remain remote Git scope.
- Relation traversal is limited to three hops. A separate result-count/truncation contract is required
  before exposing dense multi-hop graphs as an unbounded interactive surface.
- Usage recording is non-blocking for Catalog reads. Dedicated usage-delivery observability belongs in
  the operational telemetry milestone.
