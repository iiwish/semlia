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

![Semlia governed semantic answer prototype](docs/assets/semlia-product-overview.jpg)

<p align="center"><sub>Product prototype with repository-local mock data. The production application is being rebuilt against real service contracts.</sub></p>

## What is Semlia?

Semlia is an enterprise semantic asset platform and governance control plane. It gives every important business concept and metric an authoritative, machine-readable home for its definition, calculation, owner, lineage, evidence, tests, history, consumers, and AI context.

Instead of scattering meaning across SQL, dashboards, documents, and prompts, Semlia manages semantics as software assets: **discoverable, testable, reviewable, releasable, and compatibility-aware**.

Semlia is built around four ideas:

- **Living LLM Wiki**: authoritative pages connect human-readable meaning with evidence, ownership, history, and context suitable for AI.
- **Governed ontology**: concepts, entities, metrics, relationships, constraints, and physical bindings form one machine-readable model.
- **Software-grade lifecycle**: AI may propose changes, but tests, policy, evidence, and review determine what becomes published truth.
- **Trusted resolution**: CLI, MCP, REST, SDKs, and events resolve released semantics before an optional execution adapter runs a query.

Semlia is not a BI dashboard, workbook, unrestricted ChatBI product, data warehouse, or query engine. It is the semantic control plane around those systems.

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
| Product prototype | Complete north-star desktop prototype; repository-local mock data only |
| Engineering foundation | Go control server, PostgreSQL migrations, worker, embedded Web status app, contracts, CI, security gates, and release tooling |
| Production semantic product | Backend domain, API, persistence, and production Web integration are the next development phase |
| Public release | Blocked on production capabilities, self-hosting proof, security intake, compatibility, and release acceptance |

The prototype communicates product intent; it does not prove production behavior, authorization, tenant isolation, scale, or external-system safety.

## Quick Start

### Inspect the product prototype

Install the versions pinned in `.tool-versions`: Go 1.26.6, Node.js 24.15.0, pnpm 11.1.3, plus Git and GNU Make.

```bash
make doctor
make bootstrap
make prototype
```

Vite prints the local prototype URL after startup. The prototype is isolated from the production runtime and uses mock data.

### Run the engineering foundation

With Docker and Compose available:

```bash
make dev
make smoke
```

Open `http://127.0.0.1:8080` for the embedded status application. Stop the stack without deleting its PostgreSQL volume:

```bash
make dev-down
```

The smoke test is disruptive to this checkout's local Compose stack. The full setup, port rules, and recovery paths are documented in the [quickstart](docs/quickstart.md), [local development runbook](docs/operations/local-development.md), and [troubleshooting guide](docs/operations/troubleshooting.md).

## Development

The root `Makefile` is the stable developer interface:

| Command | Purpose |
| --- | --- |
| `make doctor` | Validate required tools and local capabilities |
| `make bootstrap` | Install locked Go and pnpm dependencies |
| `make prototype` | Start the inspectable product prototype |
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
prototypes/   Product design reference; never a production data path
sdk/          Generated and maintained client SDKs
tests/        Contract, integration, acceptance, smoke, and repository tests
web/          Production Web application
```

Start at the [documentation index](docs/README.md). Architecture decisions live in [ADRs](docs/adr/), while current product behavior and boundaries are defined by the [SSOT](docs/SSOT.md), not by prototype mock data.

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
