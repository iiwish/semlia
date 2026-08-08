# Semlia

Semlia is an AI-first semantic asset control plane. It helps teams discover, define, validate, review, release, and serve trusted business semantics to people, applications, and AI agents.

> Project status: pre-alpha. The product contract is confirmed; the first implementation milestone is in progress.

## Why Semlia

- Semantics are versioned assets, not prompt fragments.
- AI proposes changes; policy, evidence, and review determine what is published.
- Every production response resolves to an immutable release and revision.
- REST is the canonical service contract; MCP is the primary agent interface.
- Cube Core is the first execution integration, while the domain model remains engine-independent.

The canonical product and architecture contract lives in [docs/SSOT.md](docs/SSOT.md). The confirmed M0 plan and task graph are indexed in [docs/README.md](docs/README.md).

## Development

Prerequisites are pinned in `.tool-versions`:

- Go 1.26.3
- Node.js 24.15.0
- pnpm 11.1.3
- GNU Make, Git, and Docker

Node.js and pnpm are build-time tools for the Web workspace. The production control plane is distributed as a Go executable and does not require a Node.js runtime.

Check the local environment and install locked dependencies:

```bash
make doctor
make bootstrap
```

Run the current repository contract:

```bash
go test ./tests/repository
```

The M0 application processes are intentionally introduced by later governed tasks. `make dev` becomes available when the local integrated environment is implemented.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md) before opening a contribution. Contributions use the Developer Certificate of Origin sign-off.

## License

Semlia is licensed under the [Apache License 2.0](LICENSE).
