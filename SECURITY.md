# Security Policy

Semlia treats security reports as private, time-sensitive engineering work. The project is pre-alpha and has no supported public release yet.

## Supported Versions

No Semlia version currently receives production security support. This section will list supported release lines before the first public release candidate.

## Reporting a Vulnerability

Do not report a vulnerability through a public issue, pull request, discussion, or social channel.

GitHub private vulnerability reporting is not enabled while the repository is in Private incubation, so there is currently no public intake channel for external vulnerability reports. Do not open a public issue or disclose vulnerability details in repository discussions. This limitation must be resolved before the repository becomes public or publishes a release candidate.

Once GitHub private vulnerability reporting is enabled, include:

- The affected commit, version, component, and configuration.
- Reproduction steps or a minimal proof of concept.
- Expected and observed impact.
- Known mitigations or workarounds.
- Whether the issue is already public or under active exploitation.

Private vulnerability reporting must be enabled before the repository accepts external security reports or publishes a release candidate. Until then, the repository is not production-ready.

## Response Targets

These targets take effect once the private reporting channel is enabled:

- Acknowledge a complete report within 3 business days.
- Provide an initial severity and next-step assessment within 7 business days.
- Coordinate disclosure timing with the reporter after a fix and supported upgrade path exist.

Targets are not warranties. Complex or incomplete reports may require more time, but maintainers will communicate material status changes through the private report.

## Disclosure and Credit

Please allow maintainers a reasonable opportunity to investigate and release a fix before public disclosure. Semlia will credit reporters who request recognition and will not publish personal information without consent.

## Security Scope

The security process covers Semlia-owned source code, release artifacts, containers, dependency configuration, authentication and authorization boundaries, secret handling, tenant isolation, Agent tool permissions, and published deployment guidance.

## Automated Security Gates

`make security-check` runs the same dependency, secret, and release-container scans required for pull requests and the scheduled GitHub Actions security workflow. High or Critical findings fail the command and are retained in `build/security/` for diagnosis; findings are not silently ignored or accepted by configuration.

Remote actions and the Trivy scanner image use immutable commit or image-digest references. Dependabot version-update PRs are configured for Go modules, pnpm packages, Docker bases, and GitHub Actions; Dependabot vulnerability alerts remain a public-release gate. Release bundles include a CycloneDX SBOM and SHA-256 checksum subjects suitable for provenance attestation and signing.
