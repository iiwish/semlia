# Release Verification

## Local Candidate Status

The current delivery is the scoped local `1.0.0-rc.20260927.6` candidate under private incubation. Exact-binary browser checks, user-delegated CUA acceptance and its one-shot real-model cohort pass. Fresh frontend/browser/smoke/security/release gates are distinguished from unchanged-input Go/Node/performance evidence inherited from Round 7. Git delivery is authorized subject to passing PR CI; this is not personal user testing. [T001–T004 evidence](../README.md#交付与证据) establishes specific synthetic workflows, not public release approval or enterprise SLA. [T005 delivery](../evidence/V1-T005/summary.md) links the actual gates, candidate identity, 4/4 positive and 2/2 negative results, historical failures and limits. The unsigned manifest remains `local_candidate` with `acceptance: unreviewed`.

Local uncommitted work may produce `sourceDirty` candidate metadata. A source fingerprint binds the actual build inputs; a Git commit alone does not identify an uncommitted candidate. The bundle is not a signed hosted release. No push, tag, remote deployment, image publication or signing is implicit in local verification.

## Candidate Gates

Freeze behavior and documentation inputs, generate embedded assets through `scripts/dev/sync-web.sh`, and verify no generated drift. Run source, contracts, browser, isolated smoke, security and declared performance checks. Use a fresh owned security image tag so the default `semlia:security` image is not overwritten:

```bash
make security-check SEMLIA_SECURITY_IMAGE="semlia:security-<fresh-owned-id>"
```

This builds and scans an image; it is not the daily `make smoke` health check. Browser and Docker test cleanup must prove ownership before touching resources. Retain failures, skipped checks and limitations. Never replace a real-model gate with protocol stubs or omit failed attempts from the denominator.

Build only after the gates' source identity is fixed, using a fresh owned output directory and the exact Go toolchain from `go.mod`:

```bash
SEMLIA_VERSION=1.0.0-rc.20260927.6 \
  SEMLIA_RELEASE_DIR="build/release/<fresh-owned-directory>" \
  make release GO="$(go env GOROOT)/bin/go"
```

The release builder synchronizes embedded Web assets and checks its source fingerprint. Verify the generated manifest, staged executable/migrations, archive, external SBOM and checksums with the local bundle verifier:

```bash
go run ./scripts/release/manifest.go verify \
  -manifest "<release.json>" -archive "<archive>" -sbom "<external-sbom>" \
  -checksums "<SHA256SUMS>" -version "<exact-version>" -commit "<full-commit>"
```

Replace quoted placeholders with exact paths/identifiers from that build, not files selected from another candidate. Schema must be 33 and the executable target must match its intended host. A failed fingerprint, checksum, SBOM or schema check blocks launch; it is not a reason to rewrite metadata or force a database version.

The owned V1 acceptance entry provides `candidate-check` and `up-candidate`, each with the four arguments `<release.json> <archive> <external-sbom> <checksums>`. Validate before explicitly stopping an existing owned runtime. Candidate startup installs the exact verified executable without compilation, rechecks digests, and starts only server/worker. Its UI is served by the binary at the API URL; Vite cannot stand in for the embedded assets. Mode, actual binary digest, manifest digest and schema must remain consistent in runtime and test receipts.

`node scripts/acceptance/v1-browser.mjs candidate-preflight` uses normal-password read-only desktop checks against that verified embedded UI. The final-candidate real-model cohort is a separate, explicitly authorized command with fresh keys, fixed questions, independent SQL and strict time windows; all first attempts, refusals and failures remain visible. These owned acceptance scripts require their matching private ownership state and are not general installation commands.

## Signed Releases

For an actual separately authorized signed release, receipts use the maintainer's Ed25519 key. The trusted public key is pinned in `scripts/release/signing-public.pem`; private keys belong only in protected secret storage. Verify with the trusted source tree, not a replacement key shipped in a download:

```sh
node scripts/release/proof.mjs verify "<download-root>" "<tag>" "<full-commit>"
```

Hosted receipts bind platform bundles, SBOMs, checksums and image identity to repository/version/commit/build-run identity. Signed local receipts use `urn:semlia:local-release:<UUID>` and are not hosted attestations or independent provenance guarantees. Neither type is created merely by building this RC. Source-history rewrites do not transfer old artifact proofs to new commit identities.

## Deployment and Security

[Deployment examples](../../deploy/examples/README.md) are templates, not a configured installation or deployment authorization. Use approved database/network ownership, dedicated roles, connection budgets, protected configuration and durable paths; server and worker both require successful migration. Verify [full recovery](backup-recovery.md) before upgrading an existing installation. Evidence supports only the stated identity/workspace schema 32→33 baseline, not arbitrary legacy data.

A development-tool output disclosed private configuration. Before production, rotate affected credentials through an approved process, preserve encryption-key recovery and verify stored source credentials after any key migration. Do not put secret values, DSNs, sessions, raw provider responses or private traces in release evidence. Synthetic acceptance is not an availability, scalability or disaster-recovery SLA.
