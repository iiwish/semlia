# Semlia

> **Semlia is an AI-first data semantics platform built on Cube's executable semantic layer and a living wiki for organizational meaning.**
>
> **Semlia 是一个以 Cube Core 为可执行语义内核、以 LLM Wiki 为知识组织与协作范式的 AI-first 数据语义中台。**

Semlia turns metrics, dimensions, business concepts, relationships, evidence, and consumption contracts into governed semantic assets that people, applications, and AI agents can understand and use together.

It combines three product ideas:

- **Executable semantics:** Cube Core compiles and executes governed semantic queries, access rules, and pre-aggregations.
- **A living semantic wiki:** every semantic asset has an authoritative page for its meaning, calculation, owner, lineage, evidence, tests, history, consumers, and AI context.
- **An AI-first governance lifecycle:** AI discovers gaps and proposes structured changes; policy, validation, evidence, and review determine what becomes published truth.

> Project status: pre-alpha, under Private incubation. The M0 engineering foundation is implemented and is undergoing fresh-clone acceptance; M1 product capabilities are not implemented. The product prototype uses repository-local mock data and is isolated from the production runtime. This repository is not production-ready and does not yet operate as an active public community project.

## Product Positioning

Semlia is the semantic knowledge and governance layer around an executable semantic core. It helps teams:

1. discover existing semantics and supporting evidence;
2. define metrics and business meaning as stable assets rather than prompt fragments;
3. validate changes against schemas, references, Cube compilation, policies, and evaluation cases;
4. review AI-generated proposals without allowing models to silently rewrite production truth;
5. publish immutable semantic releases and bind consumers to explicit versions;
6. serve the same released definitions, relationships, and evidence to people, applications, and AI agents;
7. improve semantic quality from usage, failures, drift, and incidents.

Semlia is not a BI dashboard, a general data catalog, a text-to-SQL product, a data warehouse, or a new query execution engine.

## Relationship With Cube

Cube Core is Semlia's first executable semantic kernel, not a component Semlia intends to replace.

| Concern | Primary responsibility |
| --- | --- |
| Semantic model compilation and query planning | Cube Core |
| Query execution, access enforcement, and pre-aggregations | Cube Core |
| Stable semantic asset identity and living wiki pages | Semlia |
| Discovery, evidence, ownership, and relationship graph | Semlia |
| AI proposals, validation orchestration, review, and policy | Semlia |
| Immutable semantic releases, consumer bindings, and audit | Semlia |
| Governed context and released facts for agents | Semlia, delegating metric execution to Cube Core |

Semlia integrates through public Cube contracts and model artifacts. It does not fork Cube, duplicate its query runtime, or reproduce Cube's BI, dashboard, and conversational analytics surfaces. When an agent needs a metric result, Semlia resolves the governed asset, release, evidence, and policy context, then delegates execution to Cube Core through an adapter.

Cube Core is the first-priority integration, while Semlia's domain model remains engine-independent. Additional semantic runtimes can integrate through explicit adapter and validation contracts as real demand emerges.

## Product Principles

- Semantics are versioned assets, not prompt fragments.
- AI proposes changes; policy, evidence, and review determine what is published.
- Every production semantic response resolves to an immutable release and revision.
- Git stores reviewable semantic content; the product does not require users to understand Git.
- REST is the canonical service contract; MCP is the primary agent interface.
- Cube Core executes semantics; Semlia governs how semantic truth is formed, proven, released, and consumed.

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

Release output is written to the ignored `build/release/` directory. Each bundle records the complete source commit and target platform, includes migrations, notices, and its SBOM, and is accompanied by `SHA256SUMS` for verification and signing. Tagged GitHub release builds also create a provenance attestation.

## Contributing

Semlia is in Private incubation, so the repository is not currently accepting public contributions. The contribution contract is prepared for the future public phase: read [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md) before participating once the repository opens. Contributions use the Developer Certificate of Origin sign-off.

## License

Semlia is licensed under the [Apache License 2.0](LICENSE).
