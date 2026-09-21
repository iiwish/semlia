<div align="center">

# Semlia

**The open semantic asset control plane for trusted data and AI.**

Turn business concepts, metrics, relationships, evidence, and physical bindings into
versioned assets that people, applications, and AI agents can safely share.

[English](README.md) | [简体中文](README.zh-CN.md)

[![Status: pre-alpha](https://img.shields.io/badge/status-pre--alpha-EA580C)](#project-status)
[![License: Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-2563EB)](LICENSE)
[![CI](https://github.com/iiwish/semlia/actions/workflows/ci.yml/badge.svg)](https://github.com/iiwish/semlia/actions/workflows/ci.yml)

</div>

![Semlia governed semantic workspace](docs/assets/semlia-product-overview.jpg)

<p align="center"><sub>The production desktop workspace combines a live PostgreSQL semantic registry with clearly marked preview surfaces for later milestones.</sub></p>

## What is Semlia?

Semlia turns trusted semantic knowledge into answers from your data. Connect databases and documents, organize business definitions and calculation rules, confirm the knowledge, and use published semantics to constrain read-only SQL queries. Governance protects the answers without becoming the user's primary workflow.

Instead of scattering meaning across SQL, dashboards, documents, and prompts, Semlia manages semantics as software assets: **discoverable, testable, reviewable, releasable, and compatibility-aware**.

Semlia is built around four ideas:

- **Living LLM Wiki**: authoritative pages connect human-readable meaning with evidence, ownership, history, and context suitable for AI.
- **Governed ontology**: concepts, entities, metrics, relationships, constraints, and physical bindings form one machine-readable model.
- **Software-grade lifecycle**: AI may propose changes, but tests, policy, evidence, and review determine what becomes published truth.
- **Trusted resolution**: CLI, MCP, REST, SDKs, and events resolve released semantics before an optional execution adapter runs a query.

Semlia is not a BI dashboard, workbook, unrestricted ChatBI product, data warehouse, or query engine. Its first-party question-and-answer workspace and external API consumers share the same published knowledge, permissions, and execution constraints. The current delivery contract and acceptance boundaries are in [Knowledge-to-data convergence](docs/specs/knowledge-to-data/convergence.md).

## How It Works

```text
Sources              Evidence               Semantics              Governance             Delivery
Catalogs / SQL  -->  Physical graph    -->  Assets + ontology -->  Validate + review -->  REST / MCP / SDK
dbt / documents      lineage + provenance   bindings + contracts   immutable releases     optional execution
```

1. **Discover** datasets, fields, SQL lineage, existing definitions, and supporting evidence.
2. **Model** metrics, concepts, relationships, constraints, physical bindings, and join contracts as stable assets.
3. **Validate** schemas, references, SQL, grain, fanout, compatibility, authorization, and adapter capabilities.
4. **Govern** proposed changes through evidence, policy, risk routing, human review, and immutable releases.
5. **Deliver** the same released meaning to people, applications, and agents through stable headless interfaces.

For the complete product and architecture contract, read the [project SSOT](docs/SSOT.md) and the [knowledge governance architecture](docs/architecture/knowledge-governance.html).

## Project Status

> [!WARNING]
> Semlia is **pre-alpha** and under **Private incubation**. It is not production-ready and is not currently accepting public contributions.

| Surface | Current state |
| --- | --- |
| Production Web | One embedded desktop workspace; M1 workspace and Catalog flows use generated contracts and PostgreSQL |
| Semantic registry | Stable TypeIDs, immutable revisions, evidence, bounded ontology relations, discovery adapters, audit, outbox, usage, and Git projection |
| Preview surfaces | Ask, governed authoring, release orchestration, source setup, and administration preserve the accepted product experience while their backend milestones are delivered |
| Public release | Blocked on production capabilities, self-hosting proof, security intake, compatibility, and release acceptance |

M1 Catalog behavior is real and suitable for local business acceptance. Preview and session-only
surfaces do not claim persistence, authorization enforcement, or external-system effects.

## Quick Start

Install the versions pinned in `.tool-versions`: Go 1.26.6, Node.js 24.15.0, pnpm 11.1.3, plus Git and GNU Make.

With Docker and Compose available:

```bash
make doctor
make bootstrap
make dev
make smoke
```

Open the local URL printed by `make dev` for the Semlia product workspace. The live M1 Catalog supports
workspace bootstrap, search, filtering, asset creation, immutable detail, evidence, and bounded
relations. System diagnostics remain at `/status`.

Stop the stack without deleting its PostgreSQL volume:

```bash
make dev-down
```

Native development reads private overrides from `.semlia/native.env`. Knowledge confirmation requires `SEMLIA_SEMANTIC_PRODUCTION_ENABLED=true` in both server and worker environments; restart with `make dev-down` followed by `make dev` after changing it. This flag does not grant model-generation permissions, confirm business rules, publish knowledge, or configure a read-only execution source.

The full setup, port rules, and recovery paths are documented in the [quickstart](docs/quickstart.md), [local development runbook](docs/operations/local-development.md), and [troubleshooting guide](docs/operations/troubleshooting.md).

## Development

The root `Makefile` is the stable developer interface:

| Command | Purpose |
| --- | --- |
| `make doctor` | Validate required tools and local capabilities |
| `make bootstrap` | Install locked Go and pnpm dependencies |
| `make build` | Build the Semlia control-plane binary |
| `make test-repository` | Check repository structure and policy contracts |
| `make contracts-check` | Detect drift in generated API contracts |
| `make check` | Run the complete local pull-request gate |
| `make release` | Build a versioned bundle, SBOM, and checksums |

## Repository Guide

```text
api/          OpenAPI contract and generated public types
cmd/          Semlia command entry point
db/           sqlc configuration and queries
deploy/       Container and deployment assets
docs/         Product SSOT, architecture, specs, operations, and community policies
internal/     Go domain, application, adapter, and platform packages
migrations/   Versioned PostgreSQL migrations
sdk/          Generated and maintained client SDKs
tests/        Contract, integration, acceptance, smoke, and repository tests
web/          Sole production Web application and desktop product experience
```

Start at the [documentation index](docs/README.md). Architecture decisions live in [ADRs](docs/adr/), while current product behavior and boundaries are defined by the [SSOT](docs/SSOT.md).

## Roadmap

- **M0, foundation**: control server, database, jobs, contracts, local stack, CI, security, and release mechanics.
- **M1, semantic registry**: real source discovery, physical graph, semantic catalog, revisions, evidence, ownership, search, and production Web foundations.
- **M2, governed authoring**: AI proposals, validation orchestration, policy, review, and publishing.
- **M3+, distribution and continuous governance**: trusted resolution, MCP/SDK delivery, consumer bindings, feedback, drift, and an open adapter ecosystem.

Milestone scope and exit criteria are canonical in the [SSOT roadmap](docs/SSOT.md#17-路线图).

## Contributing and Security

Semlia is not yet open for public contributions. The future contribution contract is documented in [Contributing](docs/CONTRIBUTING.md), and all participation is governed by the [Code of Conduct](docs/CODE_OF_CONDUCT.md).

Do not report vulnerabilities in a public issue. Read the [Security Policy](docs/SECURITY.md) for the current private-incubation limitations and future reporting process.

## License

Semlia is licensed under the [Apache License 2.0](LICENSE). See [NOTICE](NOTICE) for attribution information.
