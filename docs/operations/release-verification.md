# Release Verification

Release receipts are signed with the maintainer's Ed25519 key. The trusted public key is pinned in `scripts/release/signing-public.pem`; private keys belong only in protected secret storage. Verify receipts using the trusted source tree, not a replacement public key bundled with a download.

```sh
node scripts/release/proof.mjs verify <download-root> <tag> <full-commit>
```

Hosted receipts bind the platform bundles, SBOMs, checksums and container image identity to the repository, version, commit and build run. Local receipts use a `urn:semlia:local-release:<UUID>` identifier and bind the image archive, image identity, SBOM and validation record. Local receipts are not hosted attestations and do not provide an independent third-party provenance guarantee.

Source-history rewrites invalidate associations with old commit identities. Rewritten historical tags are source references, not freshly verified or signed binary releases. An existing receipt only applies to its original artifact bytes and commit identity. A release from rewritten history requires a new version, fresh validation and a newly signed receipt; do not relabel an old binary or reuse its proof.

Deployment examples are in `deploy/examples/`. Operator-specific database roles, connection budgets, network names, credentials, host paths, access policies and release records belong outside this repository. The examples do not authorize a deployment or describe a running installation.
