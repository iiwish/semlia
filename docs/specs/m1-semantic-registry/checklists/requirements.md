# M1 Backend Requirements Checklist

## T001 Entry

- [x] M0 and T008 are Accepted.
- [x] M1 product and technical direction are Confirmed.
- [x] UUIDv7/TypeID prefixes and auto-increment boundaries are Confirmed.
- [x] Existing OpenAPI generation and drift checks are reusable.
- [x] Stable upstream TypeID implementation is available under an acceptable license.
- [x] T001 excludes database migration and runtime endpoints.
- [x] Wrong prefix, malformed ID, UUID version, round-trip and JSON/text boundary tests are specified.

## Milestone Requirements

- [x] Real PostgreSQL proves every schema and transaction invariant.
- [x] Source credentials remain outside domain payloads and logs.
- [x] Prior source and asset revisions remain addressable.
- [x] Every physical and semantic claim can resolve provenance and evidence.
- [x] Invalid ontology endpoints/predicates fail closed.
- [x] API cursors are opaque and ordering is deterministic.
- [x] Git projections are deterministic and replay-safe.
- [x] Usage signals contain no raw search text or customer fact rows.
- [ ] 10,000-table benchmark and golden retrieval set are recorded.
- [ ] Exact-ref acceptance, security and release gates pass.
