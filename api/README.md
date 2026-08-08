# Public Contracts

`openapi/semlia.v1.yaml` is the canonical Semlia HTTP and shared-envelope contract. Generated Go and TypeScript files are committed review artifacts, not independent sources of truth.

## Versioning

- API paths use a major version such as `v1`.
- `info.version` is the semantic version of the complete contract bundle.
- `x-semlia-contract-version` is the machine-readable contract format generation.
- Released contracts remain immutable. Breaking changes require a new major API or envelope version and an explicit migration decision.

## Change Policy

- Additive optional fields and endpoints are normally compatible.
- Removing or renaming fields, narrowing accepted values, adding required input, or changing response types is breaking.
- Public identifiers, timestamps, errors and events use shared component schemas.
- Contract files never include semantic asset fields before their product specification is confirmed.

Run `make contracts` after editing the specification and `make contracts-check` before review.
