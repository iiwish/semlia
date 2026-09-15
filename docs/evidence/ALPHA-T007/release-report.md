# Controlled-User Alpha 0.1.0 Release Report

## Artifact Set

| Target | Archive SHA-256 | SBOM SHA-256 |
| --- | --- | --- |
| Darwin arm64 | `23fef44caffcf3b81084dff026d5a801f41c5c2ec5087939924ce2479770c9ae` | `9fa3cd366a58411392d0476bb663e96c46c642bab6adcc75a0816cec97cc9620` |
| Darwin amd64 | `d9d386266581fe4f4b94036100454e093426deec8a6ebf18668bc51484e21402` | `9eac99d5bdf0e724101c54d02ed389e483e85bf31aa3c8d2353608df9ac437af` |
| Linux amd64 | `73aa31f00c5350ffd58d48f8c8b761f48fbf6e7104f29ac88aff3a8d2cc89839` | `776cf6be514e45a2364029fdfad4d960b0872d3e3b8ef5a411cf127e53113358` |
| Linux arm64 | `f8387f7c59a4524ebeb5aace5ebb4445aef9e291366d826c0eeb43128d3fea36` | `1cdd588e875af5e007e71cad4907dab1a05fc0fe6b0f5566abcf67d12278a522` |

`SHA256SUMS` verification passes for the current host archive and SBOM. Release packaging uses
`CGO_ENABLED=0`; PostgreSQL SQL-artifact parsing uses the repository's wazero-backed pg_query
implementation so every release target builds without a native parser dependency.

## Security And Runtime

- Dependency vulnerabilities: 0.
- Secret findings: 0.
- Release image Debian vulnerabilities: 0.
- Release image Go-binary vulnerabilities: 0.
- Scanned image digest: `sha256:c14c8d9caf61158b2eac702d61c822663ebb1833e6c41af8f38f6b48d6c93ab8`.
- Local acceptance image digest: `sha256:414dae922852e43c2f879e75f1f9a7f0e97dcd8119606606a49c0e0d967512bd`.
- Database migration: `15`, dirty `false`.
- Readiness: ready at `http://127.0.0.1:18081/health/ready`.

## Distribution Boundary

These artifacts are release-shaped Alpha builds, not a public-production deployment. External TLS,
OIDC/provider secrets, backup policy, monitoring destinations and hosted rollback ownership remain
deployment inputs for the controlled-user environment.
