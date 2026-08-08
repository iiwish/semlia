# Security Policy

Semlia treats security reports as private, time-sensitive engineering work. The project is pre-alpha and has no supported public release yet.

## Supported Versions

No Semlia version currently receives production security support. This section will list supported release lines before the first public release candidate.

## Reporting a Vulnerability

Do not report a vulnerability through a public issue, pull request, discussion, or social channel.

Use GitHub private vulnerability reporting for the Semlia repository. Include:

- The affected commit, version, component, and configuration.
- Reproduction steps or a minimal proof of concept.
- Expected and observed impact.
- Known mitigations or workarounds.
- Whether the issue is already public or under active exploitation.

Private vulnerability reporting must be enabled before the repository accepts external security reports or publishes a release candidate. Until then, the repository is not presented as production-ready.

## Response Targets

- Acknowledge a complete report within 3 business days.
- Provide an initial severity and next-step assessment within 7 business days.
- Coordinate disclosure timing with the reporter after a fix and supported upgrade path exist.

Targets are not warranties. Complex or incomplete reports may require more time, but maintainers will communicate material status changes through the private report.

## Disclosure and Credit

Please allow maintainers a reasonable opportunity to investigate and release a fix before public disclosure. Semlia will credit reporters who request recognition and will not publish personal information without consent.

## Security Scope

The security process covers Semlia-owned source code, release artifacts, containers, dependency configuration, authentication and authorization boundaries, secret handling, tenant isolation, Agent tool permissions, and published deployment guidance.
