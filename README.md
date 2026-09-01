# Semlia

> **Semlia is an enterprise semantic asset platform that organizes business meaning as a living LLM Wiki and governed ontology.**
>
> **Semlia 是一个以 LLM Wiki 组织企业含义、以本体表达业务概念与关系的企业语义资产平台。**

Semlia governs business semantics as executable, testable, releasable software assets with explicit consumer compatibility constraints. It turns metrics, dimensions, business concepts, relationships, evidence, contracts, and quality signals into trusted assets that people, applications, and AI agents can use through stable headless interfaces.

It combines four product ideas:

- **A living semantic wiki:** every semantic asset has an authoritative page for its meaning, calculation, owner, lineage, evidence, tests, history, consumers, and AI context.
- **A governed business ontology:** concepts, entities, metrics, relationships, constraints, and physical bindings form a shared machine-readable model of organizational meaning.
- **A software-grade asset lifecycle:** AI can discover gaps and propose changes, while tests, policy, evidence, review, immutable releases, and compatibility contracts determine what becomes published truth.
- **Trusted semantic resolution:** CLI, MCP, REST, SDKs, and events resolve released semantics to governed physical bindings and join contracts before optional execution adapters are invoked.

> Project status: pre-alpha, under Private incubation. The M0 engineering foundation is implemented and is undergoing fresh-clone acceptance; M1 product capabilities are not implemented. The product prototype uses repository-local mock data and is isolated from the production runtime. This repository is not production-ready and does not yet operate as an active public community project.

## Product Positioning

Semlia is an enterprise semantic asset platform and governance control plane around executable semantic runtimes. It helps teams:

1. discover physical datasets, fields, SQL lineage, existing semantics, and supporting evidence;
2. define metrics and business meaning as stable assets rather than prompt fragments;
3. validate changes against schemas, references, SQL, joins, grain, configured execution adapters, policies, and evaluation cases;
4. review AI-generated proposals without allowing models to silently rewrite production truth;
5. publish immutable semantic releases and bind consumers to explicit versions;
6. resolve the same released definitions, physical bindings, join contracts, and evidence for people, applications, and AI agents;
7. improve semantic quality from usage, failures, drift, and incidents.

Semlia includes Governed Ask, a first-party reference interface for questions grounded in released knowledge and semantic assets. It is not a BI dashboard, workbook, unrestricted ChatBI, general-purpose text-to-SQL product, data warehouse, or new query execution engine.

## Execution Runtime Boundary

Semlia does not require Cube to ingest warehouse metadata, build a physical graph, govern semantic assets, publish releases, or resolve a semantic query. Query execution is delegated through optional, capability-scoped adapters. Cube Core is one supported adapter for teams that already use it or need its compilation, access-control, and pre-aggregation capabilities.

| Concern | Primary responsibility |
| --- | --- |
| Warehouse catalog, DDL, SQL, lineage, keys, and grain discovery | Semlia source adapters |
| Semantic query resolution, physical binding, and join planning | Semlia |
| Plan policy, grain, fanout, compatibility, and capability validation | Semlia |
| Semantic model compilation and query execution | Optional Cube or governed warehouse adapter |
| Execution-time access enforcement and pre-aggregations | Configured execution runtime |
| Stable semantic asset identity and living wiki pages | Semlia |
| Physical graph, discovery, evidence, ownership, and relationship graph | Semlia |
| AI proposals, validation orchestration, review, and policy | Semlia |
| Immutable semantic releases, consumer bindings, and audit | Semlia |
| Governed context and released semantics for agents and applications | Semlia, delegating only validated plans when execution is required |

Semlia integrates with execution runtimes through public contracts and versioned model or query-plan artifacts. It does not fork Cube, duplicate warehouse query runtimes, or reproduce BI, dashboard, workbook, or unrestricted conversational analytics surfaces. Governed Ask and external consumers use the same released definitions, ontology context, physical bindings, join contracts, evidence, and permitted execution capabilities.

An agent cannot choose arbitrary physical tables, invent joins, or execute unrestricted SQL. Semlia first produces a release-bound resolved semantic plan and validates ambiguity, grain, cardinality, fanout, filters, authorization, compatibility, and adapter capabilities. Unresolved or unsafe plans are rejected or returned for clarification.

## Product Principles

- Semantics are versioned assets, not prompt fragments.
- Business semantics are governed like software: executable, testable, releasable, and compatibility-aware.
- AI proposes changes; policy, evidence, and review determine what is published.
- Every production semantic response resolves to an immutable release and revision.
- Git stores reviewable semantic content; the product does not require users to understand Git.
- REST is the canonical service contract; MCP is the primary agent interface.
- Execution runtimes execute validated plans; Semlia governs how physical evidence and semantic assets are discovered, bound, joined, tested, released, resolved, and delivered.

The canonical product and architecture contract lives in [docs/SSOT.md](docs/SSOT.md). The confirmed M0 plan and task graph are indexed in [docs/README.md](docs/README.md).

## Development

For a first local run, follow the [quickstart](docs/quickstart.md). Maintainers should also read the [local development runbook](docs/operations/local-development.md) and [troubleshooting guide](docs/operations/troubleshooting.md).

Prerequisites are pinned in `.tool-versions`:

- Go 1.26.5
- Node.js 24.15.0
- pnpm 11.1.3
- GNU Make, Git, and Docker

Node.js and pnpm are build-time tools for the Web workspace. The production control plane is distributed as a Go executable and does not require a Node.js runtime.

Check the local environment and install locked dependencies:

```bash
make doctor
make bootstrap
```

Run the repository and public contract checks:

```bash
make test-repository
make test-contracts
make contracts-check
```

Run the complete pull-request gate locally:

```bash
make check
```

The source, integrated smoke, and security portions are also available as `make check-source`, `make check-smoke`, and `make security-check`. CI runs those independent gates in parallel while `make check` remains the local superset.

Start or build the inspectable product prototype:

```bash
make prototype
make prototype-build
```

Start the complete M0 environment:

```bash
make dev
```

This builds one release image and starts PostgreSQL, the explicit migration step, the control server, and the worker. The server embeds the Web status application, so the full local surface is available at `http://127.0.0.1:8080`. PostgreSQL is exposed only on `127.0.0.1:5433`; the host's port `5432` is not used by Semlia.

`make dev` creates random local credentials in the ignored `.semlia/dev.env` file with owner-only permissions. `.env.example` documents the available local settings without containing usable credentials.

Verify, inspect, and stop the environment with:

```bash
make smoke
./scripts/dev/compose.sh logs --follow server worker
make dev-down
```

`make dev-down` removes the local containers and network while preserving the PostgreSQL volume for the next start.

Build a versioned release bundle and a standalone CycloneDX SBOM with:

```bash
SEMLIA_VERSION=0.1.0 make release
make sbom
```

Release output is written to the ignored `build/release/` directory. Each bundle records the complete source commit and target platform, includes migrations, notices, and its SBOM, and is accompanied by `SHA256SUMS` for verification and signing. The GitHub workflow is configured to request provenance attestation for tags; a successful tagged attestation under the current Private repository plan is not yet claimed and remains a public-release gate.

## Contributing

Semlia is in Private incubation, so the repository is not currently accepting public contributions. The contribution contract is prepared for the future public phase: read [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md) before participating once the repository opens. Contributions use the Developer Certificate of Origin sign-off.

## License

Semlia is licensed under the [Apache License 2.0](LICENSE).
