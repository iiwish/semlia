# Contributing to Semlia

Thank you for helping build a trustworthy open semantic control plane. Semlia is pre-alpha, so contributions should begin with the confirmed product contract and the task ownership rules rather than an isolated implementation idea.

## Start Here

1. Read `docs/SSOT.md` and `docs/README.md`.
2. Check the relevant confirmed plan, work graph, and task packet.
3. Discuss scope before implementing a new capability or changing a public contract.
4. Do not mix unrelated refactors into a governed task.

## Development Setup

Required tools are pinned in `.tool-versions`.

```bash
make doctor
make bootstrap
make test-repository
```

The root Make targets are the stable local interface. CI must call the same underlying commands contributors can run locally.

## Changes

- Add tests before behavior changes and record the expected failing result.
- Keep public API, event, file, CLI, and MCP contracts versioned.
- Include migration, rollback, security, accessibility, and observability evidence when the affected requirement calls for it.
- Update canonical documentation in present tense. Do not create competing definitions of product behavior.
- Never commit credentials, private customer data, personal paths, or generated local state.

## Commit Sign-off

Semlia uses the [Developer Certificate of Origin 1.1](https://developercertificate.org/). Sign each commit with:

```text
Signed-off-by: Your Name <your-email@example.com>
```

Git can add the sign-off automatically:

```bash
git commit -s
```

By signing off, you certify that you have the right to submit the contribution under the project's license.

## Review Expectations

Each change is reviewed for:

1. Compliance with the confirmed SSOT, plan, task, and allowed file scope.
2. Correctness, security, performance, maintainability, and test quality.
3. User-facing acceptance, including failure and recovery paths.

Implementation is not accepted until required validation evidence exists and a maintainer explicitly approves it.

## Community Conduct and Security

Participation is governed by `CODE_OF_CONDUCT.md`. Report vulnerabilities through the private process in `SECURITY.md`, never through a public issue.
