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

- [ ] Real PostgreSQL proves every schema and transaction invariant.
- [ ] Source credentials remain outside domain payloads and logs.
- [ ] Prior source and asset revisions remain addressable.
- [ ] Every physical and semantic claim can resolve provenance and evidence.
- [ ] Invalid ontology endpoints/predicates fail closed.
- [ ] API cursors are opaque and ordering is deterministic.
- [ ] Git projections are deterministic and replay-safe.
- [ ] Usage signals contain no raw search text or customer fact rows.
- [ ] 10,000-table benchmark and golden retrieval set are recorded.
- [ ] Exact-ref acceptance, security and release gates pass.

